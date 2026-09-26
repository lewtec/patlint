package pattern

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// GrepHit is one match ready for encoding (location + snippet + captures).
type GrepHit struct {
	File     string
	Line     int
	Col      int
	Match    string // snippet / matched text
	Captures map[string]string
}

// GrepFormatter encodes grep hits. Implementations may assume CaptureNames
// is fixed for the run (statically derived from the pattern).
type GrepFormatter interface {
	// Begin is called once before any hits (e.g. CSV header).
	Begin(w io.Writer, captureNames []string) error
	// Format writes one hit.
	Format(w io.Writer, hit GrepHit, captureNames []string) error
	// End is called after the stream (flush). Optional cleanup.
	End(w io.Writer) error
}

// NewGrepFormatter returns a formatter for name: text, csv, or jsonl.
func NewGrepFormatter(name string) (GrepFormatter, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "text":
		return &textFormatter{showVars: false}, nil
	case "csv":
		return &csvFormatter{}, nil
	case "jsonl":
		return &jsonlFormatter{}, nil
	default:
		return nil, fmt.Errorf("%w: unknown format %q (want text, csv, or jsonl)", ErrFormat, name)
	}
}

// NewTextGrepFormatter is text format; showVars prints tab-indented binds.
func NewTextGrepFormatter(showVars bool) GrepFormatter {
	return &textFormatter{showVars: showVars}
}

// --- text ---

type textFormatter struct {
	showVars bool
}

func (f *textFormatter) Begin(io.Writer, []string) error { return nil }

func (f *textFormatter) Format(w io.Writer, hit GrepHit, captureNames []string) error {
	if _, err := fmt.Fprintf(w, "%s:%d:%d: %s\n", hit.File, hit.Line, hit.Col, hit.Match); err != nil {
		return err
	}
	if !f.showVars {
		return nil
	}
	// Prefer static order from the pattern when provided.
	names := captureNames
	if len(names) == 0 {
		names = sortedKeys(hit.Captures)
	}
	for _, name := range names {
		val := hit.Captures[name]
		if val == "" && hit.Captures != nil {
			// still print declared vars (empty if unbound)
			if _, ok := hit.Captures[name]; !ok && len(captureNames) > 0 {
				// declared but missing — print empty
			}
		}
		if _, err := fmt.Fprintf(w, "\t%s=%s\n", name, hit.Captures[name]); err != nil {
			return err
		}
	}
	return nil
}

func (f *textFormatter) End(io.Writer) error { return nil }

// --- csv ---

type csvFormatter struct {
	cw *csv.Writer
}

func (f *csvFormatter) Begin(w io.Writer, captureNames []string) error {
	f.cw = csv.NewWriter(w)
	header := []string{"file", "line", "col", "match"}
	header = append(header, captureNames...)
	return f.cw.Write(header)
}

func (f *csvFormatter) Format(w io.Writer, hit GrepHit, captureNames []string) error {
	if f.cw == nil {
		if err := f.Begin(w, captureNames); err != nil {
			return err
		}
	}
	row := []string{
		hit.File,
		fmt.Sprintf("%d", hit.Line),
		fmt.Sprintf("%d", hit.Col),
		hit.Match,
	}
	// hit.Captures is already display text (PublicCaptures).
	for _, name := range captureNames {
		row = append(row, hit.Captures[name])
	}
	if err := f.cw.Write(row); err != nil {
		return err
	}
	f.cw.Flush()
	return f.cw.Error()
}

func (f *csvFormatter) End(io.Writer) error {
	if f.cw != nil {
		f.cw.Flush()
		return f.cw.Error()
	}
	return nil
}

// --- jsonl ---

type jsonlFormatter struct{}

func (f *jsonlFormatter) Begin(io.Writer, []string) error { return nil }

func (f *jsonlFormatter) Format(w io.Writer, hit GrepHit, captureNames []string) error {
	// Stable object: fixed fields + captures map (ordered keys for readability).
	caps := make(map[string]string, len(captureNames))
	if len(captureNames) > 0 {
		for _, name := range captureNames {
			caps[name] = hit.Captures[name]
		}
	} else {
		caps = hit.Captures
	}
	rec := struct {
		File     string            `json:"file"`
		Line     int               `json:"line"`
		Col      int               `json:"col"`
		Match    string            `json:"match"`
		Captures map[string]string `json:"captures"`
	}{
		File:     hit.File,
		Line:     hit.Line,
		Col:      hit.Col,
		Match:    hit.Match,
		Captures: caps,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

func (f *jsonlFormatter) End(io.Writer) error { return nil }

func sortedKeys(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		if k == "" || k == "ROOT" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
