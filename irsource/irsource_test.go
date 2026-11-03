package irsource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve stands in for both sources, with made-up indexes and files.
func serve(t *testing.T) {
	t.Helper()
	files := map[string]string{
		"/irdb/index":                          "Acme/TV/4,-1.csv\nAcme/Video Projector/0,206.csv\nZeta/DVD Player/1,2.csv\n",
		"/irdb/Acme/Video Projector/0,206.csv": "functionname,protocol,device,subdevice,function\nPOWER,NEC1,0,206,18\n",
		"/tree": `{"truncated": false, "tree": [
			{"path": "TVs", "type": "tree"},
			{"path": "TVs/Acme/Acme_55X1.ir", "type": "blob"},
			{"path": "Projectors/Acme/Series_A/Beam_2.ir", "type": "blob"},
			{"path": "TVs/Acme/README.md", "type": "blob"},
			{"path": "_Converted_/CSV/Acme/TV/4_-1.ir", "type": "blob"}]}`,
	}
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := []string{irdbBase, flipperBase, flipperTree}
	irdbBase, flipperBase, flipperTree = srv.URL+"/irdb/", srv.URL+"/flipper/", srv.URL+"/tree"
	t.Cleanup(func() { irdbBase, flipperBase, flipperTree = old[0], old[1], old[2] })
}

func TestIndexSearchFetch(t *testing.T) {
	serve(t)
	ctx := context.Background()
	irdb, err := Index(ctx, nil, IRDB)
	if err != nil {
		t.Fatal(err)
	}
	flipper, err := Index(ctx, nil, Flipper)
	if err != nil {
		t.Fatal(err)
	}
	if len(irdb) != 3 || len(flipper) != 2 {
		t.Fatalf("irdb %+v\nflipper %+v", irdb, flipper)
	}
	want := Entry{Source: IRDB, Brand: "Acme", Kind: "Video Projector", Model: "device 0, subdevice 206", Path: "Acme/Video Projector/0,206.csv"}
	if irdb[1] != want {
		t.Errorf("irdb's entry %+v", irdb[1])
	}
	if e := flipper[0]; e.Brand != "Acme" || e.Kind != "TVs" || e.Model != "Acme 55X1" || e.Name() != "Acme 55X1" {
		t.Errorf("flipper's entry %+v, named %q", e, e.Name())
	}
	if n := flipper[1].Name(); n != "Acme Beam 2" {
		t.Errorf("a series' entry named %q", n)
	}

	all := append(flipper, irdb...)
	got := Search(all, "acme projector", 0)
	if len(got) != 2 || got[0].Model != "Beam 2" || got[1].Source != IRDB {
		t.Errorf("acme projector: %+v", got)
	}
	if got := Search(all, "zeta", 0); len(got) != 1 || got[0].Name() != "Zeta DVD Player" {
		t.Errorf("zeta: %+v", got)
	}
	if got := Search(all, "acme", 1); len(got) != 1 {
		t.Errorf("acme, at most 1: %+v", got)
	}

	body, err := Fetch(ctx, nil, want)
	if err != nil || !strings.Contains(string(body), "NEC1,0,206,18") {
		t.Fatalf("fetched %q %v", body, err)
	}
	if _, err := Fetch(ctx, nil, Entry{Source: IRDB, Path: "Nope/TV/1,1.csv"}); err == nil {
		t.Error("fetched a file that is not there")
	}
}
