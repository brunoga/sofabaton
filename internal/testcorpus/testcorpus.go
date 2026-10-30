// Package testcorpus loads the captured frames under testdata/captures for
// tests.
package testcorpus

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Capture is one frame of the corpus.
type Capture struct {
	Hex      string   `json:"hex"`
	Complete bool     `json:"complete"`
	Sources  []string `json:"sources"`
	Context  string   `json:"context"`
	Note     string   `json:"note"`
}

// Bytes is the capture's raw bytes.
func (c Capture) Bytes() []byte {
	b, err := hex.DecodeString(c.Hex)
	if err != nil {
		panic(err)
	}
	return b
}

// Name identifies the capture in test output.
func (c Capture) Name() string {
	if len(c.Sources) == 0 {
		return c.Hex[:min(16, len(c.Hex))]
	}
	return filepath.Base(c.Sources[0])
}

// Upstream returns the frames extracted from the upstream project's tests.
func Upstream(tb testing.TB) []Capture {
	tb.Helper()
	var corpus struct {
		Captures []Capture `json:"captures"`
	}
	data, err := os.ReadFile(filepath.Join(Root(), "testdata", "captures", "upstream.json"))
	if err != nil {
		tb.Fatal(err)
	}
	if err := json.Unmarshal(data, &corpus); err != nil {
		tb.Fatal(err)
	}
	return corpus.Captures
}

// Complete returns the upstream frames that are exactly one valid frame.
func Complete(tb testing.TB) []Capture {
	tb.Helper()
	var out []Capture
	for _, c := range Upstream(tb) {
		if c.Complete {
			out = append(out, c)
		}
	}
	return out
}

// Root is the repository root.
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// BySource returns the upstream frame first seen at src ("path:line" at the
// pinned upstream commit).
func BySource(tb testing.TB, src string) Capture {
	tb.Helper()
	for _, c := range Upstream(tb) {
		for _, s := range c.Sources {
			if s == src {
				return c
			}
		}
	}
	tb.Fatalf("no upstream capture at %s", src)
	return Capture{}
}

// Payload is the capture's frame payload: the bytes between the opcode and
// the checksum. It does not check the frame.
func (c Capture) Payload() []byte {
	b := c.Bytes()
	if len(b) < 5 {
		return nil
	}
	return b[4 : len(b)-1]
}
