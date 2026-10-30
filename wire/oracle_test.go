package wire

import (
	"encoding/hex"
	"testing"

	"github.com/brunoga/sofabaton/internal/testcorpus"
)

func TestOracleLabels(t *testing.T) {
	var cases []struct {
		Encoding string `json:"encoding"`
		Text     string `json:"text"`
		Len      int    `json:"len"`
		Slot     string `json:"slot"`
	}
	testcorpus.Oracle(t, "labels", &cases)
	for _, c := range cases {
		enc := ASCII
		if c.Encoding == "utf-16-be" {
			enc = UTF16BE
		}
		if got := hex.EncodeToString(enc.Slot(c.Text, c.Len)); got != c.Slot {
			t.Errorf("%v %q in %d: got %s, upstream %s", enc, c.Text, c.Len, got, c.Slot)
		}
	}
}

func TestOraclePaging(t *testing.T) {
	var cases []struct {
		BodyLen int      `json:"body_len"`
		Pages   []string `json:"pages"`
	}
	testcorpus.Oracle(t, "paging", &cases)
	for _, c := range cases {
		body := make([]byte, c.BodyLen)
		for i := range body {
			body[i] = byte(i)
		}
		got := Pages(0x01, body)
		if len(got) != len(c.Pages) {
			t.Errorf("body %d: %d pages, upstream %d", c.BodyLen, len(got), len(c.Pages))
			continue
		}
		for i := range got {
			if hex.EncodeToString(got[i]) != c.Pages[i] {
				t.Errorf("body %d page %d differs", c.BodyLen, i+1)
			}
		}
	}
}
