package pattern

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/sitter/ccgo"
)

func init() {
	// mv use sites share the Rule site engine when this package is linked.
	ingest.RegisterUseSiteRenamer(UseSiteRenames)
}

// UseSiteRenames rewrites identifier leaves for symbols in sourceSet using
// RefLeafRule (@target → newLeaf) per file — the same site backbone as rewrite.
//
// Files are taken from non-alias Uses that target sourceSet. ViaImportAlias
// spans are excluded so local import aliases keep their binding names.
// Each candidate file is parsed once; rules for all targets are compiled once.
func UseSiteRenames(root string, result *project.Result, sourceSet ingest.StringSet, newLeaf string) []project.Edit {
	if result == nil || sourceSet.Len() == 0 || newLeaf == "" {
		return nil
	}

	var targets []string
	for t := range sourceSet {
		if t != "" {
			targets = append(targets, t)
		}
	}
	slices.Sort(targets)

	rules := make([]Rule, 0, len(targets))
	for _, target := range targets {
		rule, err := RefLeafRule(target, newLeaf)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return nil
	}

	// Skip import-alias binding spans so local alias names are not rewritten.
	skip := ingest.FileSpanSet{}
	files := map[string]struct{}{}
	for _, u := range result.Uses {
		if !sourceSet.Has(u.Target) || u.StartByte >= u.EndByte {
			continue
		}
		ref := ingest.ParseReference(u.Reference)
		file := strings.TrimPrefix(filepath.ToSlash(ref.Path), "./")
		if file == "" {
			continue
		}
		if u.ViaImportAlias {
			skip.MarkRange(file, u.StartByte, u.EndByte)
			continue
		}
		files[file] = struct{}{}
	}
	if len(files) == 0 {
		return nil
	}

	langByFile := map[string]string{}
	for _, f := range result.Files {
		p := strings.TrimPrefix(filepath.ToSlash(f.Path), "./")
		langByFile[p] = f.Language
	}

	var fileList []string
	for f := range files {
		fileList = append(fileList, f)
	}
	slices.Sort(fileList)

	var edits []project.Edit
	for _, file := range fileList {
		abs := file
		if !filepath.IsAbs(abs) {
			abs = lewpath.New(root, filepath.FromSlash(file)).String()
		}
		source, err := os.ReadFile(abs)
		if err != nil {
			edits = append(edits, graphUseEditsForFile(result, sourceSet, newLeaf, file)...)
			continue
		}
		lang := langByFile[file]
		if lang == "" {
			edits = append(edits, graphUseEditsForFile(result, sourceSet, newLeaf, file)...)
			continue
		}
		pf, err := ingestutil.ParseSourceFile(ccgo.Engine{}, abs, lang)
		if err != nil {
			edits = append(edits, graphUseEditsForFile(result, sourceSet, newLeaf, file)...)
			continue
		}

		for _, rule := range rules {
			_, fileEdits, err := rule.ExpandFile(project.NewSession(root).WithEngine(ccgo.Engine{}), root, file, source, pf.Root, result)
			if err != nil {
				continue
			}
			for _, e := range fileEdits {
				if skip.Overlaps(file, e.Span) {
					continue
				}
				edits = append(edits, e)
			}
		}
		pf.Close()
	}
	return edits
}

// graphUseEditsForFile is a per-file fallback matching useSiteRenamesFromGraph.
func graphUseEditsForFile(result *project.Result, sourceSet ingest.StringSet, newLeaf, file string) []project.Edit {
	var edits []project.Edit
	for _, rel := range result.Uses {
		if !sourceSet.Has(rel.Target) || rel.ViaImportAlias {
			continue
		}
		ref := ingest.ParseReference(rel.Reference)
		f := strings.TrimPrefix(filepath.ToSlash(ref.Path), "./")
		if f != file {
			continue
		}
		edits = ingest.AppendReplaceSpan(edits, file, ingestutil.Span{StartByte: rel.StartByte, EndByte: rel.EndByte}, newLeaf)
	}
	return edits
}
