package irsource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testKey stands in for the app's key: the catalog's tests encrypt with
// it, never with the real one.
const testKey = "0123456789abcdef"

// serveSofabaton stands in for the catalog: its answers encrypted with
// testKey, as the real one does with the app's.
func serveSofabaton(t *testing.T) {
	t.Helper()
	db := SofabatonDB{Key: testKey}
	answer := func(w http.ResponseWriter, v any) {
		b, _ := json.Marshal(v)
		s, _ := db.encrypt(b)
		_, _ = w.Write([]byte(s))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/pro/app/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("resource") != "android" {
			http.Error(w, "no resource", http.StatusBadRequest)
			return
		}
		plain, err := db.decrypt([]byte(r.FormValue("data")))
		if err != nil {
			// As the real one: an error, encrypted with its own key.
			answer(w, map[string]any{"code": 1, "msg": "bad data"})
			return
		}
		var req map[string]any
		_ = json.Unmarshal(plain, &req)
		switch strings.TrimPrefix(r.URL.Path, "/pro/app/") {
		case "getTypes":
			answer(w, map[string]any{"code": 0, "msg": "ok", "data": []map[string]any{{"etype_id": "2", "name": "TV"}}})
		case "getAllXBrands":
			answer(w, map[string]any{"code": 0, "msg": "ok", "data": []map[string]any{
				{"brand_id": "b1", "brand_name": "Acme "}, {"brand_id": "b1", "brand_name": "Acme"}, {"brand_id": "b2", "brand_name": "Zeta"}}})
		case "getAllModels":
			if req["brand_id"] != "b1" {
				answer(w, map[string]any{"code": 1, "msg": "no brand"})
				return
			}
			answer(w, map[string]any{"code": 0, "msg": "ok", "data": []map[string]any{
				{"ecode_id": "c1", "name": "55X1", "typename": "TV", "regular": "(?i)^55x.*"},
				{"ecode_id": "c2", "name": "Box", "typename": "Stream Box", "regular": ""}}})
		case "getBt":
			answer(w, map[string]any{"code": 0, "msg": "ok", "data": []map[string]any{
				{"mode": "1", "broadcast": "", "modelName": "", "brandName": "Android TV", "codeId": "5e0122b8"},
				{"mode": "0", "broadcast": ".*", "modelName": "(4th Gen)", "brandName": "Apple", "codeId": "afecff98"},
				{"mode": "1", "brandName": "Broken", "codeId": "not hex"}}})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/api/decode_keys/", func(w http.ResponseWriter, r *http.Request) {
		var q map[string]string
		_ = json.Unmarshal([]byte(r.URL.Query().Get("data")), &q)
		if q["ecode_id"] != "c1" || !strings.HasSuffix(r.URL.Path, "/c1") {
			answer(w, map[string]any{"code": 404, "message": "no code set"})
			return
		}
		answer(w, map[string]any{"code": 0, "message": "ok", "data": map[string]any{"keys": []map[string]any{
			{"keyname": "Power", "format": "1", "irCode": "503A4E454320523A333834303020443A3420533A32353120463A38"}}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldAPI, oldCodes := sofabatonAPI, sofabatonCodes
	sofabatonAPI, sofabatonCodes = srv.URL+"/pro/", srv.URL+"/api/decode_keys/"
	t.Cleanup(func() { sofabatonAPI, sofabatonCodes = oldAPI, oldCodes })
}

func TestSofabaton(t *testing.T) {
	serveSofabaton(t)
	ctx := context.Background()
	db := SofabatonDB{Key: testKey}
	if err := db.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (SofabatonDB{Key: "fedcba9876543210"}).Check(ctx); !errors.Is(err, ErrSofabatonKey) {
		t.Errorf("a wrong key: %v", err)
	}
	if err := (SofabatonDB{Key: "short"}).Check(ctx); !errors.Is(err, ErrSofabatonKey) {
		t.Errorf("a key of the wrong size: %v", err)
	}
	brands, err := db.Brands(ctx)
	if err != nil || len(brands) != 2 || brands[0] != (SofabatonBrand{ID: "b1", Name: "Acme"}) {
		t.Fatalf("brands %+v, %v", brands, err)
	}
	models, err := db.Models(ctx, brands[0])
	if err != nil || len(models) != 2 || models[0].Path != "c1.sofabaton" || models[0].File() != "c1.sofabaton" {
		t.Fatalf("models %+v, %v", models, err)
	}
	if models[0].Type() != "TV" || models[1].Type() != "Media player" {
		t.Errorf("types %s, %s", models[0].Type(), models[1].Type())
	}
	// A model number the pattern covers finds the model.
	if got := Search(models, "acme 55X900", 0); len(got) != 1 || got[0].Model != "55X1" {
		t.Errorf("search by model number: %+v", got)
	}
	// The model named comes before one whose pattern covers the name.
	more := append(models, Entry{Source: Sofabaton, Brand: "Acme", Kind: "TV", Model: "55X2", Pattern: "(?i)^55x1$"})
	if got := Search(more, "acme 55x1", 0); len(got) != 2 || got[0].Model != "55X1" || got[1].Model != "55X2" {
		t.Errorf("ranking: %+v", got)
	}
	if _, err := db.Models(ctx, brands[1]); err == nil || !strings.Contains(err.Error(), "no brand") {
		t.Errorf("a brand the catalog refuses: %v", err)
	}
	set, err := db.Fetch(ctx, models[0])
	if err != nil || !strings.Contains(string(set), `"keyname":"Power"`) {
		t.Fatalf("code set %s, %v", set, err)
	}
	if _, err := db.Fetch(ctx, models[1]); err == nil || !strings.Contains(err.Error(), "no code set") {
		t.Errorf("a missing code set: %v", err)
	}
	if _, err := Fetch(ctx, nil, models[0]); err == nil {
		t.Error("Fetch fetched a Sofabaton code set without the key")
	}
}

func TestSofabatonBluetooth(t *testing.T) {
	serveSofabaton(t)
	bt, err := SofabatonDB{Key: testKey}.Bluetooth(context.Background())
	if err != nil || len(bt) != 2 {
		t.Fatalf("profiles %+v, %v", bt, err)
	}
	if bt[0].Name() != "Android TV" || bt[1].Name() != "Apple (4th Gen)" || bt[1].Kind != BluetoothKind || bt[1].Path != "afecff98.sofabaton" || bt[0].Classic || !bt[1].Classic {
		t.Errorf("profiles %+v", bt)
	}
}
