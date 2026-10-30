package wire

import (
	"bytes"
	"errors"
	"testing"
)

func record(index, total byte, content []byte) [][]byte {
	body := append([]byte{total, 0, 0}, content...)
	pages := PageCount(len(body))
	body[1], body[2] = byte(pages>>8), byte(pages)
	return Pages(index, body)
}

func TestBurst(t *testing.T) {
	r1 := record(1, 3, []byte("first"))
	r2 := record(2, 3, bytes.Repeat([]byte{0xAB}, 300)) // two pages
	r3 := record(3, 3, nil)
	if len(r2) != 2 {
		t.Fatalf("record 2 has %d pages", len(r2))
	}
	var b Burst
	order := [][]byte{r2[1], r3[0], r1[0], r2[0]}
	for i, p := range order {
		done, err := b.Add(p)
		if err != nil {
			t.Fatal(err)
		}
		if done != (i == len(order)-1) {
			t.Fatalf("after %d pages done = %v", i+1, done)
		}
	}
	recs, err := b.Records()
	if err != nil || len(recs) != 3 || b.Len() != 3 {
		t.Fatalf("%d records, %v", len(recs), err)
	}
	if !bytes.Equal(recs[0][3:], []byte("first")) || len(recs[1]) != 303 || len(recs[2]) != 3 {
		t.Fatalf("records %q / %d / %d bytes", recs[0][3:], len(recs[1]), len(recs[2]))
	}
	b.Reset()
	if b.Done() || b.Len() != 0 {
		t.Fatal("Reset left state")
	}
}

func TestBurstErrors(t *testing.T) {
	var b Burst
	if _, err := b.Records(); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("Records: %v", err)
	}
	if _, err := b.Add(record(4, 3, nil)[0]); err == nil {
		t.Fatal("record 4 of 3 accepted")
	}
	if _, err := b.Add(record(0, 3, nil)[0]); err == nil {
		t.Fatal("record 0 accepted")
	}
	if _, err := b.Add(record(1, 3, nil)[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add(record(2, 4, nil)[0]); err == nil {
		t.Fatal("disagreeing record count accepted")
	}
}

func FuzzBurst(f *testing.F) {
	f.Add(record(1, 1, []byte("x"))[0], record(1, 2, nil)[0])
	f.Fuzz(func(t *testing.T, a, b []byte) {
		var burst Burst
		_, _ = burst.Add(a)
		_, _ = burst.Add(b)
		if recs, err := burst.Records(); err == nil && len(recs) != burst.Len() {
			t.Fatalf("%d records of %d", len(recs), burst.Len())
		}
	})
}
