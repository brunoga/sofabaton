package irsource

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
)

// Sofabaton is the Sofabaton app's own catalog of IR code sets: its
// brands, each brand's models (with a pattern of the model numbers they
// cover), and each model's code set, the X2's descriptors as they are
// (docs/research/README.md, 2026-11-04). It is not a public service:
// Sofabaton does not document it, and its requests and answers are
// encrypted with a key the app holds, which callers supply (SofabatonDB);
// it is not in this library.
const Sofabaton Source = "Sofabaton"

// Where the catalog is; tests point it at their own server.
var (
	sofabatonAPI   = "https://api.furrykid.com/pro/"
	sofabatonCodes = "https://api.furrykid.com/api/decode_keys/"
)

// SofabatonExt is the extension of the code sets SofabatonDB.Fetch
// returns, which irfile.Read reads.
const SofabatonExt = ".sofabaton"

// ErrSofabatonKey reports a key that does not decrypt the catalog's
// answers.
var ErrSofabatonKey = errors.New("irsource: the key does not decrypt Sofabaton's catalog")

// SofabatonDB is the Sofabaton catalog, reached with the key its answers
// are encrypted with (16 characters, AES-128).
type SofabatonDB struct {
	Key    string
	Client *http.Client
}

// codeID is a code set's id in the catalog: hex.
var codeID = regexp.MustCompile(`^[0-9a-fA-F]{1,64}$`)

// SofabatonBrand is a maker in the catalog.
type SofabatonBrand struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Check reports whether the catalog answers with the key: nil if so,
// ErrSofabatonKey if the key does not decrypt its answer.
func (db SofabatonDB) Check(ctx context.Context) error {
	var types []struct {
		ID   json.Number `json:"etype_id"`
		Name string      `json:"name"`
	}
	if err := db.post(ctx, "app/getTypes", map[string]any{"language": 1}, &types); err != nil {
		return err
	}
	if len(types) == 0 {
		return errors.New("irsource: Sofabaton's catalog lists no kinds of device")
	}
	return nil
}

// Brands are the catalog's makers.
func (db SofabatonDB) Brands(ctx context.Context) ([]SofabatonBrand, error) {
	var rows []struct {
		ID   string `json:"brand_id"`
		Name string `json:"brand_name"`
	}
	if err := db.post(ctx, "app/getAllXBrands", map[string]any{"language": 1}, &rows); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []SofabatonBrand
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if r.ID == "" || name == "" || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, SofabatonBrand{ID: r.ID, Name: name})
	}
	return out, nil
}

// Models are a brand's models, as entries: Kind is the catalog's kind of
// device ("TV", "Stream Box"), Pattern the model numbers each covers,
// Path its code set (with SofabatonExt), for Fetch.
func (db SofabatonDB) Models(ctx context.Context, b SofabatonBrand) ([]Entry, error) {
	var rows []struct {
		Code    string `json:"ecode_id"`
		Name    string `json:"name"`
		Kind    string `json:"typename"`
		Pattern string `json:"regular"`
	}
	if err := db.post(ctx, "app/getAllModels", map[string]any{"brand_id": b.ID}, &rows); err != nil {
		return nil, err
	}
	var out []Entry
	for _, r := range rows {
		if !codeID.MatchString(r.Code) {
			continue
		}
		out = append(out, Entry{Source: Sofabaton, Brand: b.Name, Kind: r.Kind, Model: strings.TrimSpace(r.Name), Path: r.Code + SofabatonExt, Pattern: r.Pattern})
	}
	return out, nil
}

// Bluetooth are the catalog's Bluetooth profiles (the app's Bluetooth
// devices: Android TV, Apple TV, Fire TV…), as entries of Kind
// "Bluetooth" whose code sets (Fetch) hold HID usages. A device the app
// makes of one carries its id as its record's code id (our X2's AWOL
// projector, 2026-11-04). Creating one needs the hub to pair with the
// device, which is not known yet.
func (db SofabatonDB) Bluetooth(ctx context.Context) ([]Entry, error) {
	var rows []struct {
		Brand string `json:"brandName"`
		Model string `json:"modelName"`
		Code  string `json:"codeId"`
		// Mode is "1" for BLE and "0" for classic Bluetooth; one row
		// has none.
		Mode any `json:"mode"`
	}
	if err := db.post(ctx, "app/getBt", map[string]any{}, &rows); err != nil {
		return nil, err
	}
	var out []Entry
	for _, r := range rows {
		brand, model := strings.TrimSpace(r.Brand), strings.TrimSpace(r.Model)
		if !codeID.MatchString(r.Code) || brand == "" {
			continue
		}
		if model == "" {
			model = brand
		}
		classic := r.Mode != nil && fmt.Sprint(r.Mode) == "0"
		out = append(out, Entry{Source: Sofabaton, Brand: brand, Kind: BluetoothKind, Model: model, Path: r.Code + SofabatonExt, Classic: classic})
	}
	return out, nil
}

// BluetoothKind is the Kind of the catalog's Bluetooth profiles.
const BluetoothKind = "Bluetooth"

// Fetch is an entry's code set, as JSON (irfile.ReadSofabaton).
func (db SofabatonDB) Fetch(ctx context.Context, e Entry) ([]byte, error) {
	code := strings.TrimSuffix(path.Base(e.Path), SofabatonExt)
	if !codeID.MatchString(code) {
		return nil, fmt.Errorf("irsource: %q is not a Sofabaton code set", e.Path)
	}
	q, _ := json.Marshal(map[string]string{"ecode_id": code})
	body, err := db.get(ctx, sofabatonCodes+url.PathEscape(code)+"?data="+url.QueryEscape(string(q)))
	if err != nil {
		return nil, err
	}
	plain, err := db.decrypt(body)
	if err != nil {
		return nil, err
	}
	var r struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(plain, &r); err != nil {
		return nil, fmt.Errorf("irsource: Sofabaton's code set: %w", err)
	}
	if r.Code != 0 || len(r.Data) == 0 || string(r.Data) == "null" {
		return nil, fmt.Errorf("irsource: Sofabaton's code set %s: %s (%d)", code, r.Message, r.Code)
	}
	return r.Data, nil
}

// post sends the catalog a request, its fields encrypted, and decodes
// the answer's data into out.
func (db SofabatonDB) post(ctx context.Context, endpoint string, fields map[string]any, out any) error {
	plain, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	data, err := db.encrypt(plain)
	if err != nil {
		return err
	}
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	if err := w.WriteField("data", data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sofabatonAPI+endpoint, &form)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("resource", "android") // as the app's
	body, err := db.do(req)
	if err != nil {
		return err
	}
	answer, err := db.decrypt(body)
	if err != nil {
		return err
	}
	var r struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(answer, &r); err != nil {
		return fmt.Errorf("irsource: Sofabaton's %s: %w", endpoint, err)
	}
	if r.Code != 0 {
		return &catalogError{endpoint: endpoint, msg: r.Msg, code: r.Code}
	}
	if err := json.Unmarshal(r.Data, out); err != nil {
		return fmt.Errorf("irsource: Sofabaton's %s: %w", endpoint, err)
	}
	return nil
}

// catalogError is the catalog's answer of a non-zero code.
type catalogError struct {
	endpoint, msg string
	code          int
}

func (e *catalogError) Error() string {
	return fmt.Sprintf("irsource: Sofabaton's %s: %s (%d)", e.endpoint, e.msg, e.code)
}

func (db SofabatonDB) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return db.do(req)
}

func (db SofabatonDB) do(req *http.Request) ([]byte, error) {
	c := db.Client
	if c == nil {
		c = http.DefaultClient
	}
	// The URL is the catalog's, with a code set's id checked as hex.
	resp, err := c.Do(req) //nolint:gosec // G704: see above

	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("irsource: Sofabaton's catalog: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

// encrypt is plain as the catalog takes it: AES-128 in ECB mode, PKCS#7
// padding, in base64.
func (db SofabatonDB) encrypt(plain []byte) (string, error) {
	b, err := aes.NewCipher([]byte(db.Key))
	if err != nil {
		return "", ErrSofabatonKey
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	in := append(append([]byte(nil), plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(in))
	for i := 0; i < len(in); i += aes.BlockSize {
		b.Encrypt(out[i:], in[i:i+aes.BlockSize])
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// decrypt is an answer of the catalog in the clear; a key that does not
// fit leaves no valid padding or JSON: ErrSofabatonKey.
func (db SofabatonDB) decrypt(answer []byte) ([]byte, error) {
	b, err := aes.NewCipher([]byte(db.Key))
	if err != nil {
		return nil, ErrSofabatonKey
	}
	in, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(answer)))
	if err != nil || len(in) == 0 || len(in)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("irsource: Sofabaton's catalog answered something not encrypted: %.60q", answer)
	}
	out := make([]byte, len(in))
	for i := 0; i < len(in); i += aes.BlockSize {
		b.Decrypt(out[i:], in[i:i+aes.BlockSize])
	}
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(out[len(out)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, ErrSofabatonKey
	}
	out = out[:len(out)-pad]
	if !json.Valid(out) {
		return nil, ErrSofabatonKey
	}
	return out, nil
}

// patterns caches compiled model patterns (nil for one that does not
// compile).
var patterns sync.Map

// matchPattern reports whether w is a model number the pattern covers.
func matchPattern(pattern, w string) bool {
	if pattern == "" {
		return false
	}
	v, ok := patterns.Load(pattern)
	if !ok {
		re, _ := regexp.Compile(pattern)
		v, _ = patterns.LoadOrStore(pattern, re)
	}
	re := v.(*regexp.Regexp)
	return re != nil && re.MatchString(w)
}
