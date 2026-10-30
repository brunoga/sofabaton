package wire

import (
	"bytes"
	"errors"
	"testing"
)

// pagedReply builds the page payloads of a reply whose body is header+content.
func pagedReply(content []byte, pages int, chunk int) [][]byte {
	body := append([]byte{0x01, byte(pages >> 8), byte(pages)}, content...)
	var out [][]byte
	for p := 1; p <= pages; p++ {
		c := body[min((p-1)*chunk, len(body)):min(p*chunk, len(body))]
		out = append(out, append([]byte{0x01, byte(p >> 8), byte(p)}, c...))
	}
	return out
}

func TestAssemblerInOrderAndShuffled(t *testing.T) {
	content := bytes.Repeat([]byte{0xAA, 0xBB, 0xCC}, 100)
	pages := pagedReply(content, 2, 247)
	want := append([]byte{0x01, 0x00, 0x02}, content...)

	for name, order := range map[string][]int{"in order": {0, 1}, "reversed": {1, 0}, "duplicate": {0, 0, 1}} {
		var a Assembler
		var done bool
		for _, i := range order {
			var err error
			if done, err = a.Add(pages[i]); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if !done || a.Pages() != 2 {
			t.Fatalf("%s: done %v pages %d", name, done, a.Pages())
		}
		got, err := a.Body()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: body %d bytes, %v", name, len(got), err)
		}
	}
}

func TestAssemblerIncompleteAndErrors(t *testing.T) {
	var a Assembler
	pages := pagedReply(make([]byte, 600), 3, 247)
	if done, _ := a.Add(pages[2]); done {
		t.Fatal("done without page 1")
	}
	if _, err := a.Body(); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("Body: %v", err)
	}
	if _, err := a.Add(pages[0]); err != nil {
		t.Fatal(err)
	}
	if a.Done() {
		t.Fatal("done without page 2")
	}
	if _, err := a.Add([]byte{0x01, 0x00, 0x04, 0x00}); err == nil {
		t.Fatal("page 4 of 3 accepted")
	}
	if _, err := a.Add([]byte{0x01, 0x00, 0x00}); err == nil {
		t.Fatal("page 0 accepted")
	}
	if _, err := a.Add([]byte{0x01, 0x00, 0x01, 0x01}); err == nil {
		t.Fatal("first page without a body header accepted")
	}
	if _, err := a.Add([]byte{0x01, 0x00, 0x01, 0x01, 0x00, 0x00}); err == nil {
		t.Fatal("0-page reply accepted")
	}
	a.Reset()
	if a.Pages() != 0 || a.Done() {
		t.Fatal("Reset left state")
	}
}

func FuzzAssembler(f *testing.F) {
	f.Add([]byte{0x01, 0x00, 0x01, 0x01, 0x00, 0x01, 0x05}, []byte{0x01, 0x00, 0x02})
	f.Fuzz(func(t *testing.T, a, b []byte) {
		var asm Assembler
		_, _ = asm.Add(a)
		_, _ = asm.Add(b)
		if body, err := asm.Body(); err == nil && len(body) < 3 {
			t.Fatalf("body of %d bytes", len(body))
		}
	})
}
