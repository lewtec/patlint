package ingest

import (
	"fmt"
	"github.com/lewtec/patlint/pkg/projectfs"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/project"
	refpkg "github.com/lewtec/patlint/pkg/reference"
)

type atomCandidate struct {
	Ref      Reference
	Language string
}

func allowDirectoryAtomRef(policy PackQueries, language string) bool {
	return languageRules(policy, language).DirectoryModule
}

func NormalizePathReference(ref Reference) Reference {
	return refpkg.NormalizePathReference(ref)
}

func languageForRefPath(result *project.Result, pathRef string) string {
	needle := strings.TrimPrefix(pathRef, "./")
	for _, f := range result.Files {
		if f.Path == needle {
			return f.Language
		}
	}
	return ""
}

func CanonicalSourceReference(dir string, result *project.Result, ref Reference, policy PackQueries) (Reference, error) {
	ref = NormalizePathReference(ref)
	if ref.Provider != "path" || ref.Name == "" {
		return ref, nil
	}

	absPath := ref.Path
	if !filepath.IsAbs(absPath) {
		absPath = lewpath.New(dir, strings.TrimPrefix(ref.Path, "./")).String()
	}

	st, err := (projectfs.OS{}).Stat(absPath)
	if err != nil || !st.IsDir() {
		return ref, nil
	}

	dirRel := CleanRelDir(ref.Path)
	prefix := dirRel
	if prefix != "" {
		prefix += "/"
	}

	langByPath := map[string]string{}
	for _, f := range result.Files {
		langByPath[f.Path] = f.Language
	}

	candidates := []atomCandidate{}
	for _, ent := range result.Atoms {
		entRef := ParseReference(ent.Reference)
		entPath := strings.TrimPrefix(entRef.Path, "./")
		if entRef.Name != ref.Name {
			continue
		}
		if prefix != "" {
			if !strings.HasPrefix(entPath, prefix) {
				continue
			}
		}
		candidates = append(candidates, atomCandidate{
			Ref:      entRef,
			Language: langByPath[entPath],
		})
	}

	if len(candidates) == 0 {
		return ref, fmt.Errorf("%w: %q under directory %q", ErrEntityNotFound, ref.Name, ref.Path)
	}
	if !hasDirectoryAtomCandidates(candidates, policy) {
		return ref, fmt.Errorf("%w: %q", ErrDirectorySymbolUnsupported, ref.String())
	}
	if len(candidates) == 1 {
		return candidates[0].Ref, nil
	}

	if picked, ok := pickPreferredDirectoryEntity(candidates, dirRel, policy); ok {
		return picked, nil
	}

	refs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		refs = append(refs, c.Ref.String())
	}
	return ref, fmt.Errorf("%w %q, matches: %s", ErrAmbiguousDirectoryRef, ref.String(), strings.Join(refs, ", "))
}

func pickPreferredDirectoryEntity(candidates []atomCandidate, dirRel string, policy PackQueries) (Reference, bool) {
	direct := []Reference{}
	for _, c := range candidates {
		if !languageRules(policy, c.Language).DirectoryModule {
			continue
		}
		candPath := strings.TrimPrefix(c.Ref.Path, "./")
		if path.Dir(candPath) == dirRel {
			direct = append(direct, c.Ref)
		}
	}
	if len(direct) == 1 {
		return direct[0], true
	}

	return Reference{}, false
}

func hasDirectoryAtomCandidates(candidates []atomCandidate, policy PackQueries) bool {
	for _, c := range candidates {
		if allowDirectoryAtomRef(policy, c.Language) {
			return true
		}
	}
	return false
}

func canonicalDestinationReference(dir string, result *project.Result, srcRef, dstRef Reference, policy PackQueries) (Reference, error) {
	dstRef = NormalizePathReference(dstRef)
	if dstRef.Provider != "path" || dstRef.Name == "" {
		return dstRef, nil
	}

	absPath := dstRef.Path
	if !filepath.IsAbs(absPath) {
		absPath = lewpath.New(dir, strings.TrimPrefix(dstRef.Path, "./")).String()
	}

	st, err := (projectfs.OS{}).Stat(absPath)
	if err != nil || !st.IsDir() {
		return dstRef, nil
	}

	srcLang := languageForRefPath(result, srcRef.Path)
	if srcLang == "" {
		return dstRef, fmt.Errorf("%w %s", ErrSourceLanguageUnknown, srcRef.String())
	}
	if !allowDirectoryAtomRef(policy, srcLang) {
		return dstRef, fmt.Errorf("%w: path %q language %q", ErrDirectoryDestUnsupported, dstRef.Path, srcLang)
	}

	base := path.Base(strings.TrimPrefix(srcRef.Path, "./"))
	if base == "" || base == "." || base == "/" {
		return dstRef, fmt.Errorf("%w: %q", ErrNoDirectoryDestMapping, srcLang)
	}
	dstRelFile := path.Join(CleanRelDir(dstRef.Path), base)
	dstRef.Path = "./" + path.Clean(dstRelFile)

	return dstRef, nil
}
