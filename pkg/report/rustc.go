package report

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/charmbracelet/lipgloss"
	"github.com/lewtec/patlint/pkg/ui/highlight"
	"github.com/mattn/go-runewidth"
)

// WriteRustc writes rustc-style diagnostics (header, snippet, carets, notes).
// Color follows highlight.AutoColor(w): TTY on, pipes/NO_COLOR off.
func WriteRustc(w io.Writer, root string, findings []Finding) error {
	return newRustcWriter(w, root, highlight.AutoColor(w)).write(findings)
}

type rustcWriter struct {
	w     io.Writer
	root  string
	color bool
	st    rustcStyle
	hl    map[string][]string
	plain map[string][]string
}

func newRustcWriter(w io.Writer, root string, color bool) *rustcWriter {
	return &rustcWriter{
		w:     w,
		root:  root,
		color: color,
		st:    newRustcStyle(w, color),
		hl:    map[string][]string{},
		plain: map[string][]string{},
	}
}

func (rw *rustcWriter) write(findings []Finding) error {
	for i, f := range findings {
		if i > 0 {
			if _, err := io.WriteString(rw.w, "\n"); err != nil {
				return err
			}
		}
		if err := rw.finding(f); err != nil {
			return err
		}
	}
	return nil
}

type rustcStyle struct {
	error, warning, note, help, gutter, minus, plus lipgloss.Style
}

func newRustcStyle(w io.Writer, color bool) rustcStyle {
	r := lipgloss.NewRenderer(w)
	r.SetColorProfile(highlight.EnvColorProfile())
	plain := r.NewStyle()
	if !color {
		return rustcStyle{
			error: plain, warning: plain, note: plain, help: plain,
			gutter: plain, minus: plain, plus: plain,
		}
	}
	return rustcStyle{
		error:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("1")),
		warning: r.NewStyle().Bold(true).Foreground(lipgloss.Color("3")),
		note:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		help:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("2")),
		gutter:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("4")),
		minus:   r.NewStyle().Foreground(lipgloss.Color("1")),
		plus:    r.NewStyle().Foreground(lipgloss.Color("2")),
	}
}

func (s rustcStyle) level(l Level) lipgloss.Style {
	switch l {
	case LevelError:
		return s.error
	case LevelWarning:
		return s.warning
	default:
		return s.note
	}
}

func (rw *rustcWriter) finding(f Finding) error {
	lvl := f.Level
	if lvl == "" {
		lvl = LevelWarning
	}
	header := rw.st.level(lvl).Render(string(lvl))
	if f.RuleID != "" {
		header += rw.st.level(lvl).Render("[" + f.RuleID + "]")
	}
	header += ": " + f.Message + "\n"
	if _, err := io.WriteString(rw.w, header); err != nil {
		return err
	}

	startLine, endLine := displayLines(f)

	loc := fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Column)
	if rw.color {
		if link := fileURL(rw.root, f.File, f.Line); link != "" {
			loc = osc8(link, loc)
		}
	}
	arrow := " " + rw.st.gutter.Render("-->") + " " + loc + "\n"
	if _, err := io.WriteString(rw.w, arrow); err != nil {
		return err
	}

	srcLines, plainLines := rw.lines(f)
	lo, hi := startLine, endLine
	if len(f.Source) > 0 {
		lo, hi = withNeighbors(startLine, endLine, displayLineCount(plainLines))
	}
	gw := gutterWidth(lo, hi)
	bar := rw.st.gutter.Render("|")

	for ln := lo; ln <= hi; ln++ {
		code, raw := lineAt(srcLines, plainLines, ln)
		if ln >= startLine && ln <= endLine {
			from, to := spanRange(f, ln, raw)
			code = underlineSpan(code, from, to)
		}
		num := rw.st.gutter.Render(fmt.Sprintf("%*d", gw, ln))
		if _, err := fmt.Fprintf(rw.w, "%s %s %s\n", num, bar, code); err != nil {
			return err
		}
	}

	return rw.notes(f, gw)
}

func (rw *rustcWriter) notes(f Finding, gw int) error {
	if f.Fixable && f.FixSkipped {
		line := " " + strings.Repeat(" ", gw) + " " + rw.st.gutter.Render("=") + " " +
			rw.st.note.Render("note") + ": fix skipped: overlap\n"
		_, err := io.WriteString(rw.w, line)
		return err
	}
	if !f.Fixable {
		return nil
	}
	line := " " + strings.Repeat(" ", gw) + " " + rw.st.gutter.Render("=") + " " +
		rw.st.help.Render("help") + ": apply with --fix\n"
	if _, err := io.WriteString(rw.w, line); err != nil {
		return err
	}
	if len(f.Source) == 0 || len(f.SiteEdits) == 0 {
		return nil
	}
	for _, e := range f.SiteEdits {
		if err := rw.diff(f.Source, e, gw); err != nil {
			return err
		}
	}
	return nil
}

func (rw *rustcWriter) diff(src []byte, e project.Edit, gw int) error {
	oldLines, newLines, startLine, ok := editLineDiff(src, e)
	if !ok {
		return nil
	}
	ln := startLine
	for _, old := range oldLines {
		num := rw.st.gutter.Render(fmt.Sprintf("%*d", gw, ln))
		if _, err := fmt.Fprintf(rw.w, "%s %s %s\n", num, rw.st.minus.Render("-"), old); err != nil {
			return err
		}
		ln++
	}
	ln = startLine
	for _, neu := range newLines {
		num := rw.st.gutter.Render(fmt.Sprintf("%*d", gw, ln))
		if _, err := fmt.Fprintf(rw.w, "%s %s %s\n", num, rw.st.plus.Render("+"), neu); err != nil {
			return err
		}
		ln++
	}
	return nil
}

func (rw *rustcWriter) lines(f Finding) (styled, raw []string) {
	if len(f.Source) == 0 {
		if f.Snippet == "" {
			return nil, nil
		}
		n := max(f.Line, 1)
		styled := make([]string, n)
		raw := make([]string, n)
		styled[n-1] = f.Snippet
		raw[n-1] = f.Snippet
		return styled, raw
	}
	key := f.File
	if _, ok := rw.plain[key]; !ok {
		rw.plain[key] = highlight.Lines(f.Source, f.File, highlight.Options{Color: false})
		if rw.color {
			rw.hl[key] = highlight.Lines(f.Source, f.File, highlight.Options{Color: true})
		} else {
			rw.hl[key] = rw.plain[key]
		}
	}
	return rw.hl[key], rw.plain[key]
}

func lineAt(styled, raw []string, line int) (code, plain string) {
	i := line - 1
	if i >= 0 && i < len(styled) {
		code = styled[i]
	}
	if i >= 0 && i < len(raw) {
		plain = raw[i]
	}
	if code == "" {
		code = plain
	}
	return code, plain
}

func displayLines(f Finding) (start, end int) {
	start = f.Line
	if start < 1 {
		start = 1
	}
	end = f.EndLine
	// Half-open span ending at column 1 of the next line is "end of previous".
	if end > start && f.EndCol <= 1 {
		end--
	}
	if end < start {
		end = start
	}
	return start, end
}

func withNeighbors(start, end, maxLine int) (lo, hi int) {
	lo = start - 1
	if lo < 1 {
		lo = 1
	}
	hi = end + 1
	if maxLine > 0 && hi > maxLine {
		hi = maxLine
	}
	if hi < end {
		hi = end
	}
	return lo, hi
}

func displayLineCount(lines []string) int {
	n := len(lines)
	if n > 0 && lines[n-1] == "" {
		return n - 1
	}
	return n
}

func spanRange(f Finding, ln int, raw string) (from, to int) {
	startLine, endLine := displayLines(f)
	from, to = 1, visualWidth(raw)+1
	if ln == startLine && f.Column > 0 {
		from = f.Column
	}
	if ln == endLine && f.EndCol > 1 {
		to = f.EndCol
	}
	if to <= from {
		to = from + 1
	}
	return from, to
}

// underlineSpan wraps the 1-based byte-column range [from, to) with SGR
// underline. Existing colors stay on: 24 turns underline off without a full reset.
func underlineSpan(s string, from, to int) string {
	before, mid, after := splitANSIByCol(s, from, to)
	if mid == "" {
		return s
	}
	return before + "\x1b[4m" + keepUnderline(mid) + "\x1b[24m" + after
}

// keepUnderline re-asserts SGR 4 after every escape so token resets
// (lipgloss \x1b[0m) do not kill underline after the first character.
func keepUnderline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := skipANSI(s, i)
			b.WriteString(s[i:j])
			b.WriteString("\x1b[4m")
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func splitANSIByCol(s string, from, to int) (before, mid, after string) {
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	var b, m, a strings.Builder
	col := 1
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := skipANSI(s, i)
			chunk := s[i:j]
			switch {
			case col < from:
				b.WriteString(chunk)
			case col < to:
				m.WriteString(chunk)
			default:
				a.WriteString(chunk)
			}
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		chunk := s[i : i+size]
		switch {
		case col < from:
			b.WriteString(chunk)
		case col < to:
			m.WriteString(chunk)
		default:
			a.WriteString(chunk)
		}
		col += size
		i += size
	}
	return b.String(), m.String(), a.String()
}

func skipANSI(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[':
		j := i + 2
		for j < len(s) {
			c := s[j]
			j++
			if c >= '@' && c <= '~' {
				break
			}
		}
		return j
	case ']':
		j := i + 2
		for j < len(s) {
			if s[j] == '\a' {
				return j + 1
			}
			if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	default:
		return i + 2
	}
}

func editLineDiff(src []byte, e project.Edit) (oldLines, newLines []string, startLine int, ok bool) {
	if int(e.EndByte) > len(src) || e.StartByte > e.EndByte {
		return nil, nil, 0, false
	}
	line, _, _, _, _, err := SpanLoc(src, e.Span)
	if err != nil {
		return nil, nil, 0, false
	}
	ls := lineStart(src, int(e.StartByte))
	le := lineEnd(src, int(e.EndByte))
	old := strings.TrimRight(string(src[ls:le]), "\n")
	prefix := string(src[ls:e.StartByte])
	suffix := strings.TrimRight(string(src[e.EndByte:le]), "\n")
	neu := prefix + e.NewText + suffix
	if old == "" && neu == "" {
		return nil, nil, 0, false
	}
	if old != "" {
		oldLines = strings.Split(old, "\n")
	}
	if neu != "" {
		newLines = strings.Split(neu, "\n")
	}
	return oldLines, newLines, line, true
}

func lineStart(src []byte, off int) int {
	if off > len(src) {
		off = len(src)
	}
	for off > 0 && src[off-1] != '\n' {
		off--
	}
	return off
}

func lineEnd(src []byte, off int) int {
	if off > len(src) {
		off = len(src)
	}
	for off < len(src) && src[off] != '\n' {
		off++
	}
	if off < len(src) {
		off++
	}
	return off
}

func gutterWidth(start, end int) int {
	n := max(start, end)
	if n < 1 {
		return 1
	}
	return len(strconv.Itoa(n))
}

func visualWidth(s string) int {
	w := 0
	for _, r := range s {
		if r == '\t' {
			w += 4 - w%4
			continue
		}
		w += runewidth.RuneWidth(r)
	}
	return w
}

func fileURL(root, file string, line int) string {
	if file == "" {
		return ""
	}
	p := file
	if root != "" && !filepath.IsAbs(file) {
		p = lewpath.New(root, filepath.FromSlash(file)).String()
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	u := url.URL{Scheme: "file", Path: abs}
	if line > 0 {
		return u.String() + "#L" + strconv.Itoa(line)
	}
	return u.String()
}

func osc8(target, text string) string {
	return "\x1b]8;;" + target + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
