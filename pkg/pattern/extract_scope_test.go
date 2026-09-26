package pattern

import (
	"testing"

	"github.com/lewtec/patlint/pkg/project"
)

func TestNestAndAssignScopes(t *testing.T) {
	fe := &project.FileExtract{
		Scopes: []project.ScopeDef{
			{StartByte: 0, EndByte: 20, Parent: 99}, // file-ish outer
			{StartByte: 5, EndByte: 15, Parent: 99}, // mid
			{StartByte: 7, EndByte: 10, Parent: 99}, // inner
			{StartByte: 5, EndByte: 15, Parent: 99}, // dup of mid
		},
		Atoms: []project.AtomDef{
			{Name: "file", StartByte: 1, EndByte: 2},
			{Name: "mid", StartByte: 6, EndByte: 7},
			{Name: "in", StartByte: 8, EndByte: 9},
		},
		Usages: []project.UsageDef{
			{Name: "u", StartByte: 8, EndByte: 9},
		},
	}
	nestAndAssignScopes(fe)
	if len(fe.Scopes) != 3 {
		t.Fatalf("scopes=%d after dedup", len(fe.Scopes))
	}
	if fe.Scopes[0].Parent != -1 {
		t.Fatalf("outer parent=%d", fe.Scopes[0].Parent)
	}
	if fe.Scopes[1].Parent != 0 {
		t.Fatalf("mid parent=%d want 0", fe.Scopes[1].Parent)
	}
	if fe.Scopes[2].Parent != 1 {
		t.Fatalf("inner parent=%d want 1", fe.Scopes[2].Parent)
	}
	if fe.Atoms[0].ScopeIdx != 0 {
		t.Fatalf("file atom idx=%d want 0", fe.Atoms[0].ScopeIdx)
	}
	if fe.Atoms[1].ScopeIdx != 1 {
		t.Fatalf("mid atom idx=%d want 1", fe.Atoms[1].ScopeIdx)
	}
	if fe.Atoms[2].ScopeIdx != 2 {
		t.Fatalf("inner atom idx=%d want 2", fe.Atoms[2].ScopeIdx)
	}
	if fe.Usages[0].ScopeIdx != 2 {
		t.Fatalf("use idx=%d want 2", fe.Usages[0].ScopeIdx)
	}
}

func TestInnermostScopeFileRoot(t *testing.T) {
	if got := innermostScope(nil, 0, 1); got != -1 {
		t.Fatalf("empty scopes idx=%d", got)
	}
	scopes := []project.ScopeDef{{StartByte: 10, EndByte: 20, Parent: -1}}
	if got := innermostScope(scopes, 0, 1); got != -1 {
		t.Fatalf("uncovered idx=%d", got)
	}
}
