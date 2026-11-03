// Package irsource finds devices' IR code files in online databases,
// irdb's and Flipper-IRDB's, and fetches them: an index of each to search
// (which apps keep, the indexes changing slowly), and a device's file
// when picked, to read with irfile.
//
// Using either database comes with its maintainers' terms: irdb's license
// asks for its notice in the product (Credit), and Flipper-IRDB's files
// are CC0 only since 2025-08-07. The files Flipper-IRDB converted from
// irdb are left out of its index.
package irsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
)

// Source is an online database of IR code files.
type Source string

const (
	// IRDB is https://github.com/probonopd/irdb: files of protocol,
	// device, subdevice and function by maker and kind of device.
	IRDB Source = "irdb"
	// Flipper is https://github.com/Lucaslhm/Flipper-IRDB: Flipper Zero's
	// files, by kind of device and maker.
	Flipper Source = "Flipper-IRDB"
)

// Sources are the databases, in the order to show them.
var Sources = []Source{Flipper, IRDB}

// Where the databases are; tests point them at their own servers.
var (
	irdbBase    = "https://cdn.jsdelivr.net/gh/probonopd/irdb@master/codes/"
	flipperBase = "https://cdn.jsdelivr.net/gh/Lucaslhm/Flipper-IRDB@main/"
	// flipperTree lists Flipper-IRDB's files: jsDelivr does not list a
	// repository that big.
	flipperTree = "https://api.github.com/repos/Lucaslhm/Flipper-IRDB/git/trees/main?recursive=1"
)

// Credit is the notice a product that uses the source shows, as its
// maintainers ask.
func Credit(s Source) string {
	switch s {
	case IRDB:
		return "Contains/accesses irdb by Simon Peter and contributors, used under permission. For licensing details and for information on how to contribute to the database, see https://github.com/probonopd/irdb"
	case Flipper:
		return "Accesses Flipper-IRDB, a collection of IR files for the Flipper Zero by its maintainers and contributors: https://github.com/Lucaslhm/Flipper-IRDB"
	}
	return ""
}

// Entry is a device's file in a source.
type Entry struct {
	Source Source `json:"source"`
	// Brand is the maker, Kind the kind of device ("TVs", "Projector"),
	// Model the device (a model, a remote's name, or for irdb the
	// protocol's device and subdevice).
	Brand string `json:"brand"`
	Kind  string `json:"kind"`
	Model string `json:"model"`
	// Path is the file's path in the source.
	Path string `json:"path"`
}

// Name is a name for the device the entry is the file of.
func (e Entry) Name() string {
	if e.Source == IRDB {
		return e.Brand + " " + e.Kind
	}
	if strings.HasPrefix(strings.ToLower(e.Model), strings.ToLower(e.Brand)) {
		return e.Model
	}
	return e.Brand + " " + e.Model
}

// File is the entry's file name, for irfile.Read.
func (e Entry) File() string { return path.Base(e.Path) }

// Index lists the files of a source.
func Index(ctx context.Context, c *http.Client, s Source) ([]Entry, error) {
	switch s {
	case IRDB:
		body, err := get(ctx, c, irdbBase+"index")
		if err != nil {
			return nil, err
		}
		return irdbIndex(string(body)), nil
	case Flipper:
		body, err := get(ctx, c, flipperTree)
		if err != nil {
			return nil, err
		}
		return flipperIndex(body)
	}
	return nil, fmt.Errorf("irsource: no source %q", s)
}

// irdbIndex is irdb's index: a path per line, maker/kind/device,subdevice.csv.
func irdbIndex(index string) []Entry {
	var out []Entry
	for _, line := range strings.Split(index, "\n") {
		p := strings.TrimSpace(line)
		parts := strings.Split(p, "/")
		if len(parts) != 3 || !strings.HasSuffix(p, ".csv") {
			continue
		}
		model := strings.TrimSuffix(parts[2], ".csv")
		if d, s, ok := strings.Cut(model, ","); ok {
			model = "device " + d
			if s != "-1" {
				model += ", subdevice " + s
			}
		}
		out = append(out, Entry{Source: IRDB, Brand: words(parts[0]), Kind: words(parts[1]), Model: model, Path: p})
	}
	return out
}

// flipperIndex is Flipper-IRDB's files, from GitHub's tree of the
// repository: kind/maker/[series/]file.ir, less the files converted from
// other databases.
func flipperIndex(tree []byte) ([]Entry, error) {
	var t struct {
		Truncated bool
		Tree      []struct{ Path, Type string }
	}
	if err := json.Unmarshal(tree, &t); err != nil {
		return nil, fmt.Errorf("irsource: Flipper-IRDB's file list: %w", err)
	}
	if t.Truncated {
		return nil, errors.New("irsource: Flipper-IRDB's file list came cut short")
	}
	var out []Entry
	for _, f := range t.Tree {
		parts := strings.Split(f.Path, "/")
		if f.Type != "blob" || !strings.HasSuffix(strings.ToLower(f.Path), ".ir") || len(parts) < 3 || strings.HasPrefix(parts[0], "_") {
			continue
		}
		model := strings.TrimSuffix(parts[len(parts)-1], path.Ext(f.Path))
		out = append(out, Entry{Source: Flipper, Kind: words(parts[0]), Brand: words(parts[1]), Model: words(model), Path: f.Path})
	}
	return out, nil
}

// words is a path's part as words: underscores as spaces.
func words(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "_", " ")), " ")
}

// Fetch is an entry's file.
func Fetch(ctx context.Context, c *http.Client, e Entry) ([]byte, error) {
	base := map[Source]string{IRDB: irdbBase, Flipper: flipperBase}[e.Source]
	if base == "" {
		return nil, fmt.Errorf("irsource: no source %q", e.Source)
	}
	segs := strings.Split(e.Path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return get(ctx, c, base+strings.Join(segs, "/"))
}

// maxSize bounds what is read: Flipper-IRDB's file list is 4 MB.
const maxSize = 32 << 20

func get(ctx context.Context, c *http.Client, u string) ([]byte, error) {
	if c == nil {
		c = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("irsource: %s: %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

// Search is the entries that have every word of query in their maker,
// kind or model, the closest first: those whose maker is a word of the
// query, then by maker and model. At most max are returned (all for 0).
func Search(entries []Entry, query string, max int) []Entry {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	type hit struct {
		e     Entry
		brand bool
	}
	var hits []hit
	for _, e := range entries {
		text := strings.ToLower(e.Brand + " " + e.Kind + " " + e.Model)
		all := true
		for _, w := range words {
			if !strings.Contains(text, w) {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		brand := false
		for _, w := range words {
			if w == strings.ToLower(e.Brand) {
				brand = true
			}
		}
		hits = append(hits, hit{e, brand})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.brand != b.brand {
			return a.brand
		}
		if a.e.Brand != b.e.Brand {
			return strings.ToLower(a.e.Brand) < strings.ToLower(b.e.Brand)
		}
		return strings.ToLower(a.e.Model) < strings.ToLower(b.e.Model)
	})
	if max > 0 && len(hits) > max {
		hits = hits[:max]
	}
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out
}
