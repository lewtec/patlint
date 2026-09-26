package pattern

import (
	"slices"

	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
	"github.com/lewtec/patlint/pkg/tape"
)

// Pack policy names the node types we care about. One iterative CST walk
// ingests that as a perception (three arenas). (node T) / under only query it.

type indexedNode struct {
	typ              string
	span             ingestutil.Span
	kidLo, kidHi     int32
	fieldLo, fieldHi int32
}

type fieldCap struct {
	name string
	span ingestutil.Span
}

type nodeIndex struct {
	nodes  []indexedNode
	kids   []int32
	fields []fieldCap
	byType map[string][]int32
	root   int32
	ok     bool
}

type indexFrame struct {
	n         *sitter.Node
	next      uint32
	cc        uint32
	typ       string
	keep      bool // type is pack-named
	childKept int32
}

func addFinderNodeTypes(f finder, dst map[string]struct{}) {
	if f == nil || dst == nil {
		return
	}
	switch x := f.(type) {
	case *finderNode:
		if x.typ != "" {
			dst[x.typ] = struct{}{}
		}
	case *finderUnder:
		addFinderNodeTypes(x.region, dst)
		addFinderNodeTypes(x.body, dst)
	case *finderTake:
		addFinderNodeTypes(x.body, dst)
	case *finderAsLang:
		addFinderNodeTypes(x.region, dst)
		addFinderNodeTypes(x.body, dst)
	case *finderOr:
		for _, a := range x.arms {
			addFinderNodeTypes(a, dst)
		}
	case *finderPred:
		addFinderNodeTypes(x.inner, dst)
	}
}

func nodeTypesFromFinder(f finder) map[string]struct{} {
	want := map[string]struct{}{}
	addFinderNodeTypes(f, want)
	return want
}

func nodeTypesFromProgram(p *ExtractProgram, rel string) map[string]struct{} {
	want := map[string]struct{}{}
	if p == nil {
		return want
	}
	for _, act := range p.Actions {
		if act.Matcher != nil && (len(act.Paths) == 0 || actionAcceptsPath(act.Paths, rel)) {
			addFinderNodeTypes(act.Matcher.root, want)
		}
		if act.Region != nil && (len(act.Paths) == 0 || actionAcceptsPath(act.Paths, rel)) {
			addFinderNodeTypes(act.Region.root, want)
		}
		if act.ScopeNode != "" {
			want[act.ScopeNode] = struct{}{}
		}
	}
	return want
}

func buildNodeIndex(root *sitter.Node, want map[string]struct{}) *nodeIndex {
	if root == nil || root.IsNull() || len(want) == 0 {
		return nil
	}
	nNodes, nKids, nFields, typeN := countIndex(root, want)
	if nNodes == 0 {
		return &nodeIndex{}
	}
	idx := &nodeIndex{
		nodes:  make([]indexedNode, nNodes),
		kids:   make([]int32, nKids),
		fields: make([]fieldCap, nFields),
		byType: make(map[string][]int32, len(typeN)),
		ok:     true,
	}
	for t, c := range typeN {
		if c > 0 {
			idx.byType[t] = make([]int32, 0, c)
		}
	}
	fillIndex(root, want, idx)
	sortByTypeDocOrder(idx)
	return idx
}

func sortByTypeDocOrder(idx *nodeIndex) {
	for _, ids := range idx.byType {
		slices.SortFunc(ids, func(a, b int32) int {
			sa, sb := idx.nodes[a].span, idx.nodes[b].span
			if sa.StartByte != sb.StartByte {
				return int(sa.StartByte) - int(sb.StartByte)
			}
			if sa.EndByte != sb.EndByte {
				return int(sb.EndByte) - int(sa.EndByte)
			}
			return 0
		})
	}
}

func countIndex(root *sitter.Node, want map[string]struct{}) (nNodes, nKids, nFields int, typeN map[string]int) {
	typeN = make(map[string]int, len(want))
	stack := []indexFrame{{n: root, cc: root.ChildCount(), typ: root.Type()}}
	stack[0].keep = wantHas(want, stack[0].typ)
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.next < f.cc {
			c := f.n.Child(f.next)
			f.next++
			if c == nil || c.IsNull() {
				continue
			}
			typ := c.Type()
			stack = append(stack, indexFrame{n: c, cc: c.ChildCount(), typ: typ, keep: wantHas(want, typ)})
			continue
		}
		kept := f.keep || f.childKept > 0
		if kept {
			nNodes++
			nKids += int(f.childKept)
			if f.keep {
				nFields += countNamedFields(f.n)
				typeN[f.typ]++
			}
		}
		stack = stack[:len(stack)-1]
		if kept && len(stack) > 0 {
			stack[len(stack)-1].childKept++
		}
	}
	return nNodes, nKids, nFields, typeN
}

func fillIndex(root *sitter.Node, want map[string]struct{}, idx *nodeIndex) {
	type fillFrame struct {
		indexFrame
		pending int // start in done[] of this node's kept children
	}
	done := make([]int32, 0, len(idx.nodes))
	stack := []fillFrame{{indexFrame: indexFrame{n: root, cc: root.ChildCount(), typ: root.Type()}}}
	stack[0].keep = wantHas(want, stack[0].typ)
	var nodeN, kidN, fieldN int32
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.next < f.cc {
			c := f.n.Child(f.next)
			f.next++
			if c == nil || c.IsNull() {
				continue
			}
			typ := c.Type()
			stack = append(stack, fillFrame{
				indexFrame: indexFrame{n: c, cc: c.ChildCount(), typ: typ, keep: wantHas(want, typ)},
				pending:    len(done),
			})
			continue
		}
		kept := f.keep || len(done) > f.pending
		if kept {
			kidLo := kidN
			for _, id := range done[f.pending:] {
				idx.kids[kidN] = id
				kidN++
			}
			done = done[:f.pending]
			fieldLo := fieldN
			typ := ""
			if f.keep {
				typ = f.typ
				fieldN = writeNamedFields(f.n, idx.fields, fieldN)
				idx.byType[typ] = append(idx.byType[typ], nodeN)
			}
			idx.nodes[nodeN] = indexedNode{
				typ:     typ,
				span:    ingestutil.Span{StartByte: f.n.StartByte(), EndByte: f.n.EndByte()},
				kidLo:   kidLo,
				kidHi:   kidN,
				fieldLo: fieldLo,
				fieldHi: fieldN,
			}
			done = append(done, nodeN)
			idx.root = nodeN
			nodeN++
		}
		stack = stack[:len(stack)-1]
	}
	idx.ok = nodeN > 0
}

func wantHas(want map[string]struct{}, typ string) bool {
	_, ok := want[typ]
	return ok
}

func countNamedFields(n *sitter.Node) int {
	if n == nil || n.IsNull() {
		return 0
	}
	var k int
	for i := uint32(0); i < n.ChildCount(); i++ {
		if n.FieldNameForChild(i) != "" {
			k++
		}
	}
	return k
}

func writeNamedFields(n *sitter.Node, dst []fieldCap, off int32) int32 {
	if n == nil || n.IsNull() {
		return off
	}
	for i := uint32(0); i < n.ChildCount(); i++ {
		name := n.FieldNameForChild(i)
		if name == "" {
			continue
		}
		c := n.Child(i)
		if c == nil || c.IsNull() {
			continue
		}
		dst[off] = fieldCap{name: name, span: ingestutil.Span{StartByte: c.StartByte(), EndByte: c.EndByte()}}
		off++
	}
	return off
}

func (idx *nodeIndex) matchAt(i int32) Match {
	n := idx.nodes[i]
	var caps map[string][]ingestutil.Span
	if n.fieldLo < n.fieldHi {
		caps = make(map[string][]ingestutil.Span, int(n.fieldHi-n.fieldLo))
		for _, f := range idx.fields[n.fieldLo:n.fieldHi] {
			caps[f.name] = append(caps[f.name], f.span)
		}
	}
	return Match{Span: n.span, Captures: caps}
}

// enclosingField is the name field of the innermost indexed node of typ
// that covers [start,end). Same answer as walking grammar ancestors; no Child.
func (idx *nodeIndex) enclosingField(start, end uint32, typ, field string, source []byte) string {
	if idx == nil || !idx.ok || typ == "" || start >= end {
		return ""
	}
	ids := idx.byType[typ]
	if len(ids) == 0 {
		return ""
	}
	// byType is start asc, end desc. Last start<=use is the first candidate;
	// scan backward; first covering span is innermost.
	lo, hi := 0, len(ids)
	for lo < hi {
		mid := (lo + hi) / 2
		if idx.nodes[ids[mid]].span.StartByte <= start {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	for i := lo - 1; i >= 0; i-- {
		n := idx.nodes[ids[i]]
		if n.span.EndByte < end {
			continue
		}
		return idx.fieldText(ids[i], field, source)
	}
	return ""
}

func (idx *nodeIndex) fieldText(id int32, field string, source []byte) string {
	n := idx.nodes[id]
	try := func(name string) string {
		if name == "" {
			return ""
		}
		for _, f := range idx.fields[n.fieldLo:n.fieldHi] {
			if f.name != name || f.span.Empty() || int(f.span.EndByte) > len(source) {
				continue
			}
			return string(source[f.span.StartByte:f.span.EndByte])
		}
		return ""
	}
	if s := try(field); s != "" {
		if field == "declarator" {
			return peelFieldIdent(s)
		}
		return s
	}
	if field != "name" {
		if s := try("name"); s != "" {
			return s
		}
	}
	if field != "declarator" {
		if s := try("declarator"); s != "" {
			return peelFieldIdent(s)
		}
	}
	return ""
}

// peelFieldIdent is the arena stand-in for deepestIdent on a C-style
// declarator field: "main()" / "*foo(int)" → the first identifier.
func peelFieldIdent(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' {
			j := i + 1
			for j < len(s) {
				c = s[j]
				if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' {
					j++
					continue
				}
				break
			}
			return s[i:j]
		}
		i++
	}
	return s
}

func (idx *nodeIndex) lookup(typ string, dom ingestutil.Span, fullFile bool) []Match {
	if idx == nil || !idx.ok || typ == "" {
		return nil
	}
	if fullFile {
		ids := idx.byType[typ]
		out := make([]Match, len(ids))
		for i, id := range ids {
			out[i] = idx.matchAt(id)
		}
		return out
	}
	out := make([]Match, 0, 8)
	stack := []int32{idx.root}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := idx.nodes[i]
		if n.span.EndByte <= dom.StartByte || n.span.StartByte >= dom.EndByte {
			continue
		}
		if n.typ == typ && spanInDomain(n.span, dom) {
			out = append(out, idx.matchAt(i))
		}
		for k := n.kidHi - 1; k >= n.kidLo; k-- {
			stack = append(stack, idx.kids[k])
		}
	}
	return out
}

// ingestPerception is one count walk + one fill walk: tape cells and the
// node index. make() sizes come from the count walk.
func ingestPerception(root *sitter.Node, source []byte, pol tape.Policy, want map[string]struct{}, targets linkTargets) ([]tok, *nodeIndex) {
	if root == nil || root.IsNull() {
		return nil, nil
	}
	nCells, nNodes, nKids, nFields, typeN := countPerception(root, source, pol, want)
	var cells []tape.Cell
	if nCells > 0 {
		cells = make([]tape.Cell, nCells)
	}
	var idx *nodeIndex
	if nNodes > 0 {
		idx = &nodeIndex{
			nodes:  make([]indexedNode, nNodes),
			kids:   make([]int32, nKids),
			fields: make([]fieldCap, nFields),
			byType: make(map[string][]int32, len(typeN)),
			ok:     true,
		}
		for t, c := range typeN {
			if c > 0 {
				idx.byType[t] = make([]int32, 0, c)
			}
		}
	}
	fillPerception(root, source, pol, want, cells, idx)
	if idx != nil {
		sortByTypeDocOrder(idx)
	}
	cells = tape.FinishLeaves(cells, source, tapeTargets(targets))
	return cellsToTok(cells), idx
}

func tapeTargets(t linkTargets) map[tape.Span]string {
	if len(t) == 0 {
		return nil
	}
	return t
}

func cellsToTok(cells []tape.Cell) []tok {
	if len(cells) == 0 {
		return nil
	}
	out := make([]tok, len(cells))
	for i, c := range cells {
		out[i] = tok{
			Span:   ingestutil.Span{StartByte: c.StartByte, EndByte: c.EndByte},
			target: c.Target,
		}
	}
	return out
}

type percFrame struct {
	indexFrame
	tapeMute bool
	muteKids bool
}

func countPerception(root *sitter.Node, source []byte, pol tape.Policy, want map[string]struct{}) (nCells, nNodes, nKids, nFields int, typeN map[string]int) {
	typeN = make(map[string]int, len(want))
	_, emit, mute := pol.TapeVisit(root, source)
	stack := []percFrame{{
		indexFrame: indexFrame{n: root, cc: root.ChildCount(), typ: root.Type(), keep: wantHas(want, root.Type())},
		muteKids:   mute,
	}}
	if emit {
		nCells++
	}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.next < f.cc {
			c := f.n.Child(f.next)
			f.next++
			if c == nil || c.IsNull() {
				continue
			}
			typ := c.Type()
			childMute := f.tapeMute || f.muteKids
			var cemitted bool
			var cmute bool
			if !childMute {
				_, cemitted, cmute = pol.TapeVisit(c, source)
				if cemitted {
					nCells++
				}
			}
			stack = append(stack, percFrame{
				indexFrame: indexFrame{n: c, cc: c.ChildCount(), typ: typ, keep: wantHas(want, typ)},
				tapeMute:   childMute,
				muteKids:   cmute,
			})
			continue
		}
		kept := f.keep || f.childKept > 0
		if kept {
			nNodes++
			nKids += int(f.childKept)
			if f.keep {
				nFields += countNamedFields(f.n)
				typeN[f.typ]++
			}
		}
		stack = stack[:len(stack)-1]
		if kept && len(stack) > 0 {
			stack[len(stack)-1].childKept++
		}
	}
	return nCells, nNodes, nKids, nFields, typeN
}

func fillPerception(root *sitter.Node, source []byte, pol tape.Policy, want map[string]struct{}, cells []tape.Cell, idx *nodeIndex) {
	type fillFrame struct {
		percFrame
		pending int
	}
	var cellN int
	writeCell := func(n *sitter.Node, muted bool) (muteKids bool) {
		if muted {
			return false
		}
		cell, emit, mute := pol.TapeVisit(n, source)
		if emit && cellN < len(cells) {
			cells[cellN] = cell
			cellN++
		}
		return mute
	}
	rootMute := writeCell(root, false)
	if idx == nil {
		// tape only: still walk so cellN matches count (muteKids stops tape, but
		// we must visit the same nodes as countPerception).
		stack := []percFrame{{
			indexFrame: indexFrame{n: root, cc: root.ChildCount()},
			muteKids:   rootMute,
		}}
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next < f.cc {
				c := f.n.Child(f.next)
				f.next++
				if c == nil || c.IsNull() {
					continue
				}
				childMute := f.tapeMute || f.muteKids
				cmute := writeCell(c, childMute)
				stack = append(stack, percFrame{
					indexFrame: indexFrame{n: c, cc: c.ChildCount()},
					tapeMute:   childMute,
					muteKids:   cmute,
				})
				continue
			}
			stack = stack[:len(stack)-1]
		}
		return
	}
	done := make([]int32, 0, len(idx.nodes))
	stack := []fillFrame{{
		percFrame: percFrame{
			indexFrame: indexFrame{n: root, cc: root.ChildCount(), typ: root.Type(), keep: wantHas(want, root.Type())},
			muteKids:   rootMute,
		},
	}}
	var nodeN, kidN, fieldN int32
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.next < f.cc {
			c := f.n.Child(f.next)
			f.next++
			if c == nil || c.IsNull() {
				continue
			}
			typ := c.Type()
			childMute := f.tapeMute || f.muteKids
			cmute := writeCell(c, childMute)
			stack = append(stack, fillFrame{
				percFrame: percFrame{
					indexFrame: indexFrame{n: c, cc: c.ChildCount(), typ: typ, keep: wantHas(want, typ)},
					tapeMute:   childMute,
					muteKids:   cmute,
				},
				pending: len(done),
			})
			continue
		}
		kept := f.keep || len(done) > f.pending
		if kept {
			kidLo := kidN
			for _, id := range done[f.pending:] {
				idx.kids[kidN] = id
				kidN++
			}
			done = done[:f.pending]
			fieldLo := fieldN
			typ := ""
			if f.keep {
				typ = f.typ
				fieldN = writeNamedFields(f.n, idx.fields, fieldN)
				idx.byType[typ] = append(idx.byType[typ], nodeN)
			}
			idx.nodes[nodeN] = indexedNode{
				typ:     typ,
				span:    ingestutil.Span{StartByte: f.n.StartByte(), EndByte: f.n.EndByte()},
				kidLo:   kidLo,
				kidHi:   kidN,
				fieldLo: fieldLo,
				fieldHi: fieldN,
			}
			done = append(done, nodeN)
			idx.root = nodeN
			nodeN++
		}
		stack = stack[:len(stack)-1]
	}
	idx.ok = nodeN > 0
}
