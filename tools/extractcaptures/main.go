// Command extractcaptures collects the hub frames that the upstream Python
// project embeds in its tests and writes them to a JSON corpus.
//
// The upstream sources are read as text and never executed. A frame is a
// string literal whose hex digits start with the A5 5A sync, joined with the
// literals Python concatenates implicitly (adjacent literals with only
// whitespace and comments between them). Frames built with expressions are
// kept too, marked incomplete.
//
// Usage:
//
//	go run ./tools/extractcaptures -upstream <checkout> -commit <sha> > testdata/captures/upstream.json
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Capture is one frame found in the upstream tests.
type Capture struct {
	Hex string `json:"hex"`
	// Complete is true when the bytes are exactly one frame: the length
	// the opcode announces and a valid checksum.
	Complete bool `json:"complete"`
	// Sources lists every place the frame appears, as path:line.
	Sources []string `json:"sources"`
	// Context is the enclosing test function or assigned name at the first
	// source, and Note the comment just above it, when there is one.
	Context string `json:"context,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Corpus is the file written to testdata.
type Corpus struct {
	Upstream string     `json:"upstream"`
	Commit   string     `json:"commit"`
	Captures []*Capture `json:"captures"`
}

func main() {
	upstream := flag.String("upstream", "", "path to a checkout of m3tac0de/home-assistant-sofabaton-x1s")
	commit := flag.String("commit", "", "upstream commit the checkout is at")
	flag.Parse()
	if *upstream == "" || *commit == "" {
		fmt.Fprintln(os.Stderr, "extractcaptures: -upstream and -commit are required")
		os.Exit(2)
	}

	corpus, err := extract(os.DirFS(*upstream), "tests")
	if err != nil {
		fmt.Fprintln(os.Stderr, "extractcaptures:", err)
		os.Exit(1)
	}
	corpus.Upstream = "github.com/m3tac0de/home-assistant-sofabaton-x1s"
	corpus.Commit = *commit

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(corpus); err != nil {
		fmt.Fprintln(os.Stderr, "extractcaptures:", err)
		os.Exit(1)
	}
}

func extract(fsys fs.FS, root string) (*Corpus, error) {
	byHex := map[string]*Capture{}
	var order []string

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".py" {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(src), "\n")
		for _, lit := range joinedLiterals(string(src)) {
			h, ok := frameHex(lit.text)
			if !ok {
				continue
			}
			where := fmt.Sprintf("%s:%d", path, lit.line)
			if c, seen := byHex[h]; seen {
				c.Sources = append(c.Sources, where)
				continue
			}
			c := &Capture{
				Hex:      h,
				Complete: completeFrame(h),
				Sources:  []string{where},
				Context:  contextAt(lines, lit.line),
				Note:     commentAbove(lines, lit.line),
			}
			byHex[h] = c
			order = append(order, h)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	corpus := &Corpus{}
	for _, h := range order {
		corpus.Captures = append(corpus.Captures, byHex[h])
	}
	sort.SliceStable(corpus.Captures, func(i, j int) bool {
		return corpus.Captures[i].Sources[0] < corpus.Captures[j].Sources[0]
	})
	return corpus, nil
}

type literal struct {
	text string
	line int // 1-based line of the first literal in the run
}

// joinedLiterals scans Python source and returns every run of adjacent
// single-line string literals, concatenated. Triple-quoted strings and
// anything that is not a literal end a run.
func joinedLiterals(src string) []literal {
	var out []literal
	var cur strings.Builder
	curLine, inRun := 0, false
	line := 1

	flush := func() {
		if inRun {
			out = append(out, literal{text: cur.String(), line: curLine})
		}
		cur.Reset()
		inRun = false
	}

	for i := 0; i < len(src); {
		c := src[i]
		switch c {
		case '\n':
			line++
			i++
		case ' ', '\t', '\r', '\\':
			i++
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case '"', '\'':
			if strings.HasPrefix(src[i:], strings.Repeat(string(c), 3)) {
				flush()
				end := strings.Index(src[i+3:], strings.Repeat(string(c), 3))
				if end < 0 {
					return out
				}
				line += strings.Count(src[i:i+3+end+3], "\n")
				i += 3 + end + 3
				continue
			}
			j := i + 1
			for j < len(src) && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			if !inRun {
				inRun, curLine = true, line
			}
			if j <= len(src) {
				cur.WriteString(src[i+1 : min(j, len(src))])
			}
			i = j + 1
		default:
			// A string prefix (b"", r"", f"") belongs to the literal that
			// follows; anything else ends the run.
			if (c == 'b' || c == 'r' || c == 'f' || c == 'B' || c == 'R' || c == 'F') &&
				i+1 < len(src) && (src[i+1] == '"' || src[i+1] == '\'') &&
				(i == 0 || !isIdent(src[i-1])) {
				i++
				continue
			}
			flush()
			i++
		}
	}
	flush()
	return out
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

var hexOnly = regexp.MustCompile(`^[0-9a-fA-F]+$`)

// frameHex returns the literal's bytes as lowercase hex when it is hex that
// starts with the frame sync.
func frameHex(s string) (string, bool) {
	s = strings.NewReplacer(" ", "", "0x", "", ",", "").Replace(s)
	if len(s) < 4 || len(s)%2 != 0 || !hexOnly.MatchString(s) {
		return "", false
	}
	s = strings.ToLower(s)
	return s, strings.HasPrefix(s, "a55a")
}

func completeFrame(h string) bool {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) < 5 || len(b) != 5+int(b[2]) {
		return false
	}
	var sum byte
	for _, x := range b[:len(b)-1] {
		sum += x
	}
	return sum == b[len(b)-1]
}

var (
	defRe    = regexp.MustCompile(`^\s*(?:async\s+)?def\s+(\w+)`)
	assignRe = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?::[^=]+)?=`)
)

// contextAt names the code a literal belongs to: an assignment on its own
// line or just above, else the enclosing function.
func contextAt(lines []string, line int) string {
	for i := line - 1; i >= 0 && i >= line-3; i-- {
		if m := assignRe.FindStringSubmatch(lines[i]); m != nil {
			if fn := enclosingDef(lines, i); fn != "" && fn != m[1] {
				return fn + "/" + m[1]
			}
			return m[1]
		}
	}
	return enclosingDef(lines, line-1)
}

func enclosingDef(lines []string, idx int) string {
	for i := idx; i >= 0; i-- {
		if m := defRe.FindStringSubmatch(lines[i]); m != nil {
			return m[1]
		}
	}
	return ""
}

// commentAbove returns the comment on the literal's line or the run of
// comment lines directly above it.
func commentAbove(lines []string, line int) string {
	if line-1 < len(lines) {
		if i := strings.Index(lines[line-1], "#"); i >= 0 && !strings.Contains(lines[line-1][:i], "\"") {
			return strings.TrimSpace(lines[line-1][i+1:])
		}
	}
	var parts []string
	for i := line - 2; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "#") {
			break
		}
		parts = append([]string{strings.TrimSpace(strings.TrimLeft(t, "#:"))}, parts...)
	}
	return strings.Join(parts, " ")
}
