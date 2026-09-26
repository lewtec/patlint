package python_test

import (
	"os"
	"strings"
	"testing"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/internal/prelude"
	"github.com/lewtec/patlint/pkg/pattern"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/walker"

	"github.com/lewtec/patlint/pkg/ingest"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func TestPythonResidualSameFileImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "helpers.py").String(), "def helper():\n    return 1\n\ndef other():\n    return helper()\n")
	mustWrite(t, lewpath.New(dir, "utils.py").String(), "def existing():\n    pass\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./utils.py::helper")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	helpers := mustRead(t, lewpath.New(dir, "helpers.py").String())
	utils := mustRead(t, lewpath.New(dir, "utils.py").String())
	if !strings.Contains(helpers, "from utils import helper") {
		t.Fatalf("helpers missing residual import:\n%s", helpers)
	}
	if !strings.Contains(helpers, "return helper()") {
		t.Fatalf("helpers lost residual use:\n%s", helpers)
	}
	if !strings.Contains(utils, "def helper():") {
		t.Fatalf("utils missing moved helper:\n%s", utils)
	}
}

// Package-root residual import (ingest_roots = package dir, e.g. boltons):
// after moving a name between sibling modules, the source file must import with
// a same-package relative form so "python -m" / installed package loads work.
func TestPythonResidualPackageRootImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "helpers.py").String(), "def helper():\n    return 1\n\ndef other():\n    return helper()\n")
	mustWrite(t, lewpath.New(dir, "utils.py").String(), "def existing():\n    pass\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./utils.py::helper")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	helpers := mustRead(t, lewpath.New(dir, "helpers.py").String())
	utils := mustRead(t, lewpath.New(dir, "utils.py").String())
	if !strings.Contains(helpers, "from .utils import helper") {
		t.Fatalf("helpers missing package-relative residual import:\n%s", helpers)
	}
	if strings.Contains(helpers, "from utils import helper") {
		t.Fatalf("helpers used bare top-level residual import:\n%s", helpers)
	}
	if !strings.Contains(helpers, "return helper()") {
		t.Fatalf("helpers lost residual use:\n%s", helpers)
	}
	if !strings.Contains(utils, "def helper():") {
		t.Fatalf("utils missing moved helper:\n%s", utils)
	}
}

func TestPythonLocalDepImportNewModule(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "models.py").String(), "class Config:\n    pass\n\ndef create_config(name):\n    return Config(name)\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./models.py::create_config", "path:./factory.py::create_config")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	factory := mustRead(t, lewpath.New(dir, "factory.py").String())
	models := mustRead(t, lewpath.New(dir, "models.py").String())
	if !strings.Contains(factory, "from models import Config") {
		t.Fatalf("factory missing local-dep import:\n%s", factory)
	}
	if !strings.Contains(factory, "def create_config") {
		t.Fatalf("factory missing moved func:\n%s", factory)
	}
	if strings.Contains(models, "def create_config") {
		t.Fatalf("models still has create_config:\n%s", models)
	}
}

func TestPythonLocalDepImportPackageRoot(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "models.py").String(), "class Config:\n    pass\n\ndef create_config(name):\n    return Config(name)\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./models.py::create_config", "path:./factory.py::create_config")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	factory := mustRead(t, lewpath.New(dir, "factory.py").String())
	if !strings.Contains(factory, "from .models import Config") {
		t.Fatalf("factory missing package-relative local-dep import:\n%s", factory)
	}
	if strings.Contains(factory, "from models import Config") {
		t.Fatalf("factory used bare top-level local-dep import:\n%s", factory)
	}
}

func TestPythonClassCrossFileResidualImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "models.py").String(), "class Config:\n    def __init__(self, name):\n        self.name = name\n\ndef create_config(name):\n    return Config(name)\n")
	mustWrite(t, lewpath.New(dir, "types.py").String(), "pass\n")
	mustWrite(t, lewpath.New(dir, "app.py").String(), "from models import Config\n\nc = Config(\"test\")\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./models.py::Config", "path:./types.py::Config")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	models := mustRead(t, lewpath.New(dir, "models.py").String())
	app := mustRead(t, lewpath.New(dir, "app.py").String())
	if !strings.Contains(models, "from types import Config") {
		t.Fatalf("models missing residual import:\n%s", models)
	}
	if !strings.Contains(app, "from types import Config") {
		t.Fatalf("app import not rewritten:\n%s", app)
	}
}

// Boltons catalog seed 347: move _TEXT_OPENFLAGS = os.O_RDWR | … from fileutils
// into debugutils left NameError: os is not defined (stdlib import not co-moved).
func TestPythonMoveConstantCoMovesStdlibImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "fileutils.py").String(), ""+
		"import os\n"+
		"\n"+
		"_TEXT_OPENFLAGS = os.O_RDWR | os.O_CREAT | os.O_EXCL\n"+
		"_BIN_OPENFLAGS = _TEXT_OPENFLAGS\n"+
		"\n"+
		"def open_flags():\n"+
		"    return _TEXT_OPENFLAGS\n")
	mustWrite(t, lewpath.New(dir, "debugutils.py").String(), ""+
		"import sys\n"+
		"\n"+
		"def trace():\n"+
		"    return sys.version\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./fileutils.py::_TEXT_OPENFLAGS", "path:./debugutils.py::_TEXT_OPENFLAGS")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	debug := mustRead(t, lewpath.New(dir, "debugutils.py").String())
	fileu := mustRead(t, lewpath.New(dir, "fileutils.py").String())
	if !strings.Contains(debug, "import os") {
		t.Fatalf("debugutils missing co-moved os import:\n%s", debug)
	}
	if !strings.Contains(debug, "_TEXT_OPENFLAGS = os.O_RDWR") {
		t.Fatalf("debugutils missing moved constant:\n%s", debug)
	}
	if !strings.Contains(fileu, "_TEXT_OPENFLAGS") {
		t.Fatalf("fileutils lost residual use:\n%s", fileu)
	}
	if !strings.Contains(fileu, "debugutils import _TEXT_OPENFLAGS") && !strings.Contains(fileu, ".debugutils import _TEXT_OPENFLAGS") {
		t.Fatalf("fileutils missing residual reverse import of _TEXT_OPENFLAGS:\n%s", fileu)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Boltons catalog seed 353: renaming ObjectInputType.get_entry while sibling
// InputTypes also define get_entry must co-rename siblings and rewrite untyped
// _data_type.get_entry call sites (else AttributeError).
func TestPythonRenameExpandsPolymorphicMethodLeaf(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "tableutils.py").String(), ""+
		"class InputType:\n"+
		"    def get_entry_seq(self, data_seq, headers):\n"+
		"        return [self.get_entry(entry, headers) for entry in data_seq]\n"+
		"\n"+
		"class DictInputType(InputType):\n"+
		"    def get_entry(self, obj, headers):\n"+
		"        return [obj.get(h) for h in headers]\n"+
		"\n"+
		"class ObjectInputType(InputType):\n"+
		"    def get_entry(self, obj, headers):\n"+
		"        return [getattr(obj, h, None) for h in headers]\n"+
		"\n"+
		"def from_data(data, headers):\n"+
		"    _data_type = ObjectInputType()\n"+
		"    return [_data_type.get_entry(data, headers)]\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./tableutils.py::ObjectInputType.get_entry", "path:./tableutils.py::ObjectInputType.fuzz_ad9bd")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, lewpath.New(dir, "tableutils.py").String())
	if strings.Contains(got, "def get_entry(") {
		t.Fatalf("sibling get_entry defs not co-renamed:\n%s", got)
	}
	if !strings.Contains(got, "def fuzz_ad9bd(") {
		t.Fatalf("renamed method missing:\n%s", got)
	}
	if strings.Contains(got, ".get_entry(") {
		t.Fatalf("call sites still use get_entry:\n%s", got)
	}
	if !strings.Contains(got, "self.fuzz_ad9bd(") || !strings.Contains(got, "_data_type.fuzz_ad9bd(") {
		t.Fatalf("expected rewritten call sites:\n%s", got)
	}
	if strings.Count(got, "def fuzz_ad9bd(") < 2 {
		t.Fatalf("expected at least 2 co-renamed method defs:\n%s", got)
	}
}

// Boltons catalog seed 397: renaming Stats.min rewrote the property so
// delattr(Stats, '_calc_' + attr_name) looked for _calc_<new> while the
// helper stayed _calc_min → AttributeError at import.
// Boltons catalog seed 1787126122: renaming SpooledIOBase.close rewrote
// doc.close() where doc = ZipFile(...) (stdlib) under unique-leaf →
// AttributeError: 'ZipFile' object has no attribute 'fuzz*'.
func TestPythonRenamePreservesStdlibZipFileClose(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "ioutils.py").String(), ""+
		"class SpooledIOBase:\n"+
		"    def close(self):\n"+
		"        return self.buffer.close()\n"+
		"\n"+
		"class SpooledBytesIO(SpooledIOBase):\n"+
		"    def __init__(self):\n"+
		"        self.buffer = None\n")
	mustWrite(t, lewpath.New(dir, "test_ioutils.py").String(), ""+
		"from zipfile import ZipFile, ZIP_DEFLATED\n"+
		"from ioutils import SpooledBytesIO\n"+
		"\n"+
		"def test_zip_compat():\n"+
		"    spooled_flo = SpooledBytesIO()\n"+
		"    doc = ZipFile(spooled_flo, 'w', ZIP_DEFLATED)\n"+
		"    doc.writestr('content.txt', 'test')\n"+
		"    doc.close()\n"+
		"    spooled_flo.close()\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./ioutils.py::SpooledIOBase.close", "path:./ioutils.py::SpooledIOBase.fuzz3e12b")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(lewpath.New(dir, "test_ioutils.py").String())
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "doc.fuzz3e12b()") {
		t.Fatalf("rewrote ZipFile.close:\n%s", text)
	}
	if !strings.Contains(text, "doc.close()") {
		t.Fatalf("expected doc.close() preserved:\n%s", text)
	}
	if !strings.Contains(text, "spooled_flo.fuzz3e12b()") {
		t.Fatalf("expected SpooledBytesIO.close renamed:\n%s", text)
	}
	src, err := os.ReadFile(lewpath.New(dir, "ioutils.py").String())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "def fuzz3e12b(self):") {
		t.Fatalf("expected declaration renamed:\n%s", src)
	}
}

func TestPythonRenameRejectCalcPrefixProperty(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "statsutils.py").String(), ""+
		"class _StatsProperty:\n"+
		"    def __init__(self, name, func):\n"+
		"        self.name = name\n"+
		"        self.func = func\n"+
		"\n"+
		"class Stats:\n"+
		"    def _calc_min(self):\n"+
		"        return min(self.data)\n"+
		"    min = _StatsProperty('min', _calc_min)\n"+
		"\n"+
		"for attr_name, attr in list(Stats.__dict__.items()):\n"+
		"    if isinstance(attr, _StatsProperty):\n"+
		"        delattr(Stats, '_calc_' + attr_name)\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Rename(t.Context(), dir, "path:./statsutils.py::Stats.min", "path:./statsutils.py::Stats.fuzz82abe")
	if err == nil {
		t.Fatal("expected unsupported rename for _calc_ + property pattern")
	}
	if !strings.Contains(err.Error(), "_calc_") {
		t.Fatalf("expected _calc_ in error, got: %v", err)
	}
}

// Module constant used by extracted function + residual reverse import → circular.
// Boltons seed 19: _parse_wraps_expected → new module left NO_DEFAULT undefined.
func TestPythonLocalDepConstantWithResidualCircular(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "funcutils.py").String(), ""+
		"try:\n"+
		"    NO_DEFAULT = object()\n"+
		"except Exception:\n"+
		"    NO_DEFAULT = object()\n"+
		"\n"+
		"def _parse_wraps_expected(expected):\n"+
		"    if isinstance(expected, str):\n"+
		"        return [(expected, NO_DEFAULT)]\n"+
		"    return []\n"+
		"\n"+
		"def wraps(expected=None):\n"+
		"    return _parse_wraps_expected(expected)\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Rename(t.Context(), dir, "path:./funcutils.py::_parse_wraps_expected", "path:./funcutils_fuzz.py::_parse_wraps_expected")
	if err == nil {
		t.Fatal("expected circular-import unsupported error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Fatalf("expected circular error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NO_DEFAULT") {
		t.Fatalf("expected NO_DEFAULT in local deps, got: %v", err)
	}
}

// Module-level residual call (graph Uses often miss these). Boltons catalog
// seed 777: statsutils::_get_conv_func moved while a module-level loop still
// called it — NameError without reverse import or circular reject.
func TestPythonModuleLevelResidualImport(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "helpers.py").String(), ""+
		"def helper():\n"+
		"    return 1\n"+
		"\n"+
		"for _ in range(1):\n"+
		"    x = helper()\n")
	mustWrite(t, lewpath.New(dir, "utils.py").String(), "def existing():\n    pass\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.Rename(t.Context(), dir, "path:./helpers.py::helper", "path:./utils.py::helper")
	if err != nil {
		t.Fatal(err)
	}
	if err := ingest.ApplyPlan(t.Context(), dir, plan); err != nil {
		t.Fatal(err)
	}
	helpers := mustRead(t, lewpath.New(dir, "helpers.py").String())
	if !strings.Contains(helpers, "from .utils import helper") && !strings.Contains(helpers, "from utils import helper") {
		t.Fatalf("helpers missing residual import for module-level use:\n%s", helpers)
	}
	if !strings.Contains(helpers, "x = helper()") {
		t.Fatalf("helpers lost residual use:\n%s", helpers)
	}
}

// Dest already imports source: residual reverse import would cycle at load.
// Boltons: socketutils imports Stats from statsutils; extracting _get_conv_func
// the other way with residual uses must refuse (seed RFT_FUZZY_SEED=777).
func TestPythonResidualDestImportsSourceCircular(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, lewpath.New(dir, "__init__.py").String(), "")
	mustWrite(t, lewpath.New(dir, "statsutils.py").String(), ""+
		"class Stats:\n"+
		"    pass\n"+
		"\n"+
		"def _get_conv_func(attr_name):\n"+
		"    def stats_helper(data, default=0.0):\n"+
		"        return getattr(Stats(), attr_name)\n"+
		"    return stats_helper\n"+
		"\n"+
		"for attr_name in (\"mean\",):\n"+
		"    func = _get_conv_func(attr_name)\n"+
		"    globals()[attr_name] = func\n")
	mustWrite(t, lewpath.New(dir, "socketutils.py").String(), "from .statsutils import Stats\n\ndef other():\n    return 1\n")

	vm, err := pattern.New(prelude.FS)
	if err != nil {
		t.Fatal(err)
	}
	w, err := walker.NewWalker(project.NewSession(dir).WithEngine(ccgo.Engine{}), vm)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Rename(t.Context(), dir, "path:./statsutils.py::_get_conv_func", "path:./socketutils.py::_get_conv_func")
	if err == nil {
		t.Fatal("expected circular-import unsupported error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Fatalf("expected circular error, got: %v", err)
	}
}
