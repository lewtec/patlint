package ingest_test

import (
	"strings"
	"testing"

	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestPruneNamedUnusedFromExtract(t *testing.T) {
	usedLine := `(import "./used")`
	deadLine := `(import "./dead")`
	src := []byte(usedLine + "\n" + deadLine + "\n(used)\n")
	deadStart := len(usedLine) + 1
	fe := &project.FileExtract{
		Imports: []project.ImportDef{
			{LocalName: "used", SourcePath: "./used", StartByte: 0, EndByte: uint32(len(usedLine))},
			{LocalName: "dead", SourcePath: "./dead", StartByte: uint32(deadStart), EndByte: uint32(deadStart + len(deadLine))},
		},
	}
	edits := ingest.PruneNamedUnusedFromExtract("x.dsl", src, fe, ingest.PruneImportOpts{})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if strings.Contains(got, "./dead") {
		t.Fatalf("dead remains: %q", got)
	}
	if !strings.Contains(got, "./used") {
		t.Fatalf("used dropped: %q", got)
	}
}

func TestPruneNamedUnusedFromExtract_GoImportBlock(t *testing.T) {
	src := []byte("package p\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nfunc f() {\n\tfmt.Println()\n\t_ = strings.TrimSpace(\"\")\n}\n")
	fmtPath := strings.Index(string(src), `"fmt"`)
	strPath := strings.Index(string(src), `"strings"`)
	use := strings.Index(string(src), "fmt.Println()")
	if fmtPath < 0 || strPath < 0 || use < 0 {
		t.Fatal("fixture")
	}
	fe := &project.FileExtract{
		Imports: []project.ImportDef{
			{LocalName: "fmt", SourcePath: "fmt", StartByte: uint32(fmtPath), EndByte: uint32(fmtPath + len(`"fmt"`))},
			{LocalName: "strings", SourcePath: "strings", StartByte: uint32(strPath), EndByte: uint32(strPath + len(`"strings"`))},
		},
	}
	edits := ingest.PruneNamedUnusedFromExtract("p.go", src, fe, ingest.PruneImportOpts{
		MaskSpans:      []ingestutil.Span{{StartByte: uint32(use), EndByte: uint32(use + len("fmt.Println()"))}},
		OnlyCandidates: []string{"fmt"},
	})
	if len(edits) == 0 {
		t.Fatal("expected prune of fmt")
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if strings.Contains(got, `"fmt"`) {
		t.Fatalf("fmt should be pruned:\n%s", got)
	}
	if !strings.Contains(got, `"strings"`) {
		t.Fatalf("strings must remain:\n%s", got)
	}
}

func TestPruneNamedUnusedFromExtract_GoImportBlockKeepsUsedNeighbor(t *testing.T) {
	src := []byte("package p\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\nfunc f() {\n\t_ = context.Background()\n\tfmt.Println()\n}\n")
	use := strings.Index(string(src), "context.Background()")
	if use < 0 {
		t.Fatal("fixture")
	}
	ctxLine := strings.Index(string(src), "\t\"context\"\n")
	fmtLine := strings.Index(string(src), "\t\"fmt\"\n")
	fe := &project.FileExtract{
		Imports: []project.ImportDef{
			{LocalName: "context", SourcePath: "context", StartByte: uint32(ctxLine), EndByte: uint32(ctxLine + len("\t\"context\"\n"))},
			{LocalName: "fmt", SourcePath: "fmt", StartByte: uint32(fmtLine), EndByte: uint32(fmtLine + len("\t\"fmt\"\n"))},
		},
	}
	edits := ingest.PruneNamedUnusedFromExtract("p.go", src, fe, ingest.PruneImportOpts{
		MaskSpans: []ingestutil.Span{{StartByte: uint32(use), EndByte: uint32(use + len("context.Background()"))}},
	})
	got := string(project.ApplyEditsInMemory(src, edits))
	if strings.Contains(got, `"context"`) {
		t.Fatalf("context should be pruned:\n%s", got)
	}
	if !strings.Contains(got, `"fmt"`) {
		t.Fatalf("fmt must remain:\n%s", got)
	}
}

func TestEnsureImportAfterLastMarked(t *testing.T) {
	src := []byte("(import \"./old\")\n")
	pathTok := `"./old"`
	ps := strings.Index(string(src), pathTok)
	fe := &project.FileExtract{
		Imports: []project.ImportDef{{
			LocalName:     "old",
			SourcePath:    "./old",
			StartByte:     0,
			EndByte:       uint32(len("(import \"./old\")")),
			PathStartByte: uint32(ps),
			PathEndByte:   uint32(ps + len(pathTok)),
		}},
	}
	edits := ingest.EnsureImportAfterLastMarked("x.dsl", src, fe, []ingest.ImportNeed{{ImportPath: "./new"}})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if !strings.Contains(got, `"./new"`) || !strings.Contains(got, `"./old"`) {
		t.Fatalf("got %q", got)
	}
}

func TestEnsureImportFirst(t *testing.T) {
	src := []byte("package p\n\nfunc f() {}\n")
	pkg := "p"
	ps := strings.Index(string(src), pkg)
	fe := &project.FileExtract{Package: "p", PackageEnd: uint32(ps + len(pkg))}
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	edits := ingest.EnsureImportFirst("p.go", src, fe, vm.ImportLineInfo("go"), vm, []ingest.ImportNeed{{ImportPath: "fmt"}})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if !strings.Contains(got, "import \"fmt\"") {
		t.Fatalf("got %q", got)
	}
	if !strings.HasPrefix(got, "package p\n") {
		t.Fatalf("package lost: %q", got)
	}
}

func TestEnsureImportFirst_CIncludeAfterGuard(t *testing.T) {
	src := []byte("#ifndef FOO_H\n#define FOO_H\n\nclass Foo {};\n\n#endif\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "foo.h", "c")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "c", pf.Root, src, "foo.h")
	if err != nil {
		t.Fatal(err)
	}
	if fe.PackageEnd == 0 {
		t.Fatal("as-package did not mark include guard")
	}
	edits := ingest.EnsureImportFirst("foo.h", src, fe, vm.ImportLineInfo("c"), vm, []ingest.ImportNeed{{ImportPath: "bar.h"}})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if !strings.Contains(got, "#define FOO_H") || !strings.Contains(got, "#include \"bar.h\"") {
		t.Fatalf("got %q", got)
	}
	if i, j := strings.Index(got, "#define FOO_H"), strings.Index(got, "#include"); i < 0 || j < i {
		t.Fatalf("include before guard: %q", got)
	}
}

func TestEnsureImportFirst_CIncludeAfterPragmaOnce(t *testing.T) {
	src := []byte("#pragma once\n\nclass Foo {};\n")
	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	pf, err := ingestutil.ParseSource(ccgo.Engine{}, src, "foo.h", "c")
	if err != nil {
		t.Fatal(err)
	}
	defer pf.Close()
	fe, err := vm.Extract(t.Context(), project.NewSession(".").WithEngine(ccgo.Engine{}), "c", pf.Root, src, "foo.h")
	if err != nil {
		t.Fatal(err)
	}
	if fe.PackageEnd == 0 {
		t.Fatal("as-package did not mark pragma once")
	}
	edits := ingest.EnsureImportFirst("foo.h", src, fe, vm.ImportLineInfo("c"), vm, []ingest.ImportNeed{{ImportPath: "bar.h"}})
	if len(edits) != 1 {
		t.Fatalf("edits=%d", len(edits))
	}
	got := string(project.ApplyEditsInMemory(src, edits))
	if !strings.HasPrefix(got, "#pragma once\n") || !strings.Contains(got, "#include \"bar.h\"") {
		t.Fatalf("got %q", got)
	}
	if strings.HasPrefix(got, "#include") {
		t.Fatalf("include before pragma: %q", got)
	}
}
