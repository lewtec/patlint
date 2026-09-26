package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"

	"github.com/lewtec/patlint/pkg/datalog"
	"github.com/lewtec/patlint/pkg/project"
	"github.com/lewtec/patlint/pkg/store"
)

// planPackageMove handles directory/package level moves (inter-package).
//
// Single pair + free destination: DirMove renames the tree on disk (pulls
// untracked/non-ingested co-located files), then Edits rewrite imports inside
// the moved tree (at new paths) and in consumer/support files.
//
// Multi-root (e.g. Java) or merge into an existing destination: file-level
// truncate/create relocation of ingested files only.
func planPackageMove(ctx context.Context, dir string, st *store.Store, result *project.Result, src, destination Reference, policy PackQueries) (project.Plan, error) {
	sourceDirectory := strings.TrimPrefix(src.Path, "./")
	dstDir := strings.TrimPrefix(destination.Path, "./")
	if sourceDirectory == "" || dstDir == "" {
		return project.Plan{}, ErrPackageMoveEmptyPaths
	}
	if sourceDirectory == dstDir {
		return project.Plan{}, nil // no-op; avoids pointless I/O + self-overwrite edits
	}
	oldDirBase := LastPathComponent(sourceDirectory)
	newDirBase := LastPathComponent(dstDir)
	dirPairs := [][2]string{{sourceDirectory, dstDir}}
	useDirMove := len(dirPairs) == 1 && canDirectoryRename(dir, dirPairs[0][0], dirPairs[0][1])
	slog.Debug("planPackageMove", "sourceDirectory", sourceDirectory, "dstDir", dstDir, "dir_pairs", len(dirPairs), "dir_move", useDirMove)

	var plan project.Plan
	movedFiles := map[string]bool{}
	nRelocated := 0
	var holes, places [][]string
	var treeEdits []project.Edit

	if useDirMove {
		fromDir, toDir := dirPairs[0][0], dirPairs[0][1]
		plan.DirMoves = []project.DirMove{{From: fromDir, To: toDir}}
		pairSrc := Reference{Provider: "path", Path: "./" + fromDir}
		pairDst := Reference{Provider: "path", Path: "./" + toDir}
		for _, f := range result.Files {
			if err := ctx.Err(); err != nil {
				return project.Plan{}, err
			}
			rel := strings.TrimPrefix(f.Path, "./")
			if !isUnderDirectory(rel, fromDir) {
				continue
			}
			if movedFiles[rel] {
				continue
			}
			movedFiles[rel] = true
			nRelocated++
			under := strings.TrimPrefix(rel, fromDir)
			under = strings.TrimPrefix(under, "/")
			dstFile := toDir
			if under != "" {
				dstFile = path.Join(toDir, under)
			}
			content, err := os.ReadFile(lewpath.New(dir, rel).String())
			if err != nil {
				return project.Plan{}, fmt.Errorf("reading %s: %w", rel, err)
			}
			treeEdits = append(treeEdits, RewriteImportsInFile(policy, dstFile, content, result, pairSrc.String(), pairDst.String())...)
		}
		slog.Debug("planPackageMove: dir move relocate", "count", nRelocated, "from", fromDir, "to", toDir)
	} else {
		for _, pair := range dirPairs {
			fromDir, toDir := pair[0], pair[1]
			pairSrc := Reference{Provider: "path", Path: "./" + fromDir}
			pairDst := Reference{Provider: "path", Path: "./" + toDir}
			for _, f := range result.Files {
				if err := ctx.Err(); err != nil {
					return project.Plan{}, err
				}
				rel := strings.TrimPrefix(f.Path, "./")
				if !isUnderDirectory(rel, fromDir) {
					continue
				}
				srcFile := rel
				if movedFiles[srcFile] {
					continue
				}
				movedFiles[srcFile] = true
				nRelocated++
				under := strings.TrimPrefix(srcFile, fromDir)
				under = strings.TrimPrefix(under, "/")
				dstFile := toDir
				if under != "" {
					dstFile = path.Join(toDir, under)
				}
				content, err := os.ReadFile(lewpath.New(dir, srcFile).String())
				if err != nil {
					return project.Plan{}, fmt.Errorf("reading %s: %w", srcFile, err)
				}
				rewrites := RewriteImportsInFile(policy, dstFile, content, result, pairSrc.String(), pairDst.String())
				rewrites = append(rewrites, RebaseMarkedImports(srcFile, dstFile, content, result)...)
				newContent := string(project.ApplyEditsInMemory(content, rewrites))
				holes = append(holes, []string{srcFile, "0", store.Itoa(uint32(len(content)))})
				destEnd := uint32(0)
				if dstBody, err := os.ReadFile(lewpath.New(dir, filepath.FromSlash(dstFile)).String()); err == nil {
					destEnd = uint32(len(dstBody))
				}
				places = append(places, []string{dstFile, "0", store.Itoa(destEnd), newContent})
			}
		}
		slog.Debug("planPackageMove: file-level relocated", "count", nRelocated)
	}

	consumers := map[string]bool{}
	for _, pair := range dirPairs {
		for c := range packageMoveConsumerFiles(result, dir, pair[0], movedFiles) {
			consumers[c] = true
		}
	}

	oldKey, newKey := importRewriteKeys(src, destination)
	if st == nil {
		return project.Plan{}, fmt.Errorf("rename: nil store")
	}
	st.Reset(store.RelationEdit)
	host := datalog.StoreHost{
		Store:     st,
		MoveHole:  holes,
		MovePlace: places,
		RewriteSpec: func(file, specifier string) (string, bool) {
			if np := RewriteImportPathFile(file, specifier, sourceDirectory, dstDir, policy); np != "" && np != specifier {
				return np, true
			}
			if specifierLooksRelative(specifier) {
				resolved := importSpecifier(specifier).resolveFrom(file)
				if resolved == "" || !isUnderDirectory(resolved, sourceDirectory) {
					return "", false
				}
			}
			np := RewriteImportPathDir(specifier, oldKey, newKey)
			if np == "" || np == specifier {
				return "", false
			}
			return np, true
		},
		NotMoveFile: func(file string) bool {
			return !movedFiles[strings.TrimPrefix(file, "./")]
		},
	}
	if err := datalog.Eval(ctx, st, datalog.MoveProgram(), host); err != nil {
		return project.Plan{}, err
	}
	for _, e := range treeEdits {
		st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
	}
	for _, e := range RewritePackageJSONPaths(dir, sourceDirectory, dstDir) {
		st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
	}
	if path.Ext(sourceDirectory) != "" && path.Ext(dstDir) != "" {
		oldMod := strings.ReplaceAll(trimSpecifierExtension(strings.TrimPrefix(sourceDirectory, "./")), "/", ".")
		newMod := strings.ReplaceAll(trimSpecifierExtension(strings.TrimPrefix(dstDir, "./")), "/", ".")
		oldStem := fileStem(sourceDirectory)
		newStem := fileStem(dstDir)
		pkg := importFileDir(sourceDirectory)
		if oldMod != newMod && result != nil {
			for _, f := range result.Files {
				if err := ctx.Err(); err != nil {
					return project.Plan{}, err
				}
				rel := strings.TrimPrefix(f.Path, "./")
				if movedFiles[rel] {
					continue
				}
				content, err := os.ReadFile(lewpath.New(dir, rel).String())
				if err != nil {
					continue
				}
				for _, e := range FindAllWholeWordOccurrences(rel, content, oldMod, newMod) {
					st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
				}
				if pkg != "" && oldStem != newStem {
					for _, e := range rewriteFromPackageModuleImport(rel, content, pkg, oldStem, newStem, result) {
						st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
					}
				}
			}
		}
	}

	rewritten := map[string]bool{}
	for _, t := range st.Rows(store.RelationEdit) {
		if len(t) > 0 {
			rewritten[strings.TrimPrefix(t[0], "./")] = true
		}
	}
	pathOld, pathNew := oldDirBase, newDirBase
	if cp := CommonPathPrefix(sourceDirectory, dstDir); cp != "" {
		if r := strings.Trim(strings.TrimPrefix(dstDir, cp), "/"); r != "" {
			pathNew = r
		}
	}
	for consumerFile := range consumers {
		if err := ctx.Err(); err != nil {
			return project.Plan{}, err
		}
		if rewritten[consumerFile] || pathOld == pathNew {
			continue
		}
		fcontent, err := os.ReadFile(lewpath.New(dir, consumerFile).String())
		if err != nil {
			return project.Plan{}, fmt.Errorf("reading %s: %w", consumerFile, err)
		}
		for _, e := range FindAllWholeWordOccurrences(consumerFile, fcontent, pathOld, pathNew) {
			st.Insert(store.RelationEdit, store.Tuple{e.File, store.Itoa(e.StartByte), store.Itoa(e.EndByte), e.NewText})
		}
	}

	plan.Edits = deduplicateEdits(editsFromStore(st))
	slog.Debug("planPackageMove: done", "dir_moves", len(plan.DirMoves), "edits", len(plan.Edits))
	return plan, nil
}

func isUnderDirectory(p, dir string) bool {
	d := strings.TrimSuffix(dir, "/")
	if d == "" {
		return false
	}
	if p == d || strings.HasPrefix(p, d+"/") {
		return true
	}
	return false
}

// packageMoveConsumerFiles returns project-relative files (not under the moved
// package tree) that have Uses or Aliases targeting something under sourceDirectory.
// Matching is graph-based: path: refs under the tree, or language import paths
// via PackageImportMatcher. Linear in graph size.
func packageMoveConsumerFiles(result *project.Result, rootDir, sourceDirectory string, movedFiles map[string]bool) map[string]bool {
	if result == nil {
		return nil
	}
	sourceDirectory = CleanRelDir(sourceDirectory)
	targets := targetsUnderPackageDir(result, rootDir, sourceDirectory)
	if len(targets) == 0 {
		return nil
	}
	consumers := map[string]bool{}
	addConsumer := func(filePath string) {
		c := CleanRelDir(filePath)
		if c == "" || movedFiles[c] || isUnderDirectory(c, sourceDirectory) {
			return
		}
		consumers[c] = true
	}
	for _, u := range result.Uses {
		if targets[u.Target] {
			addConsumer(ParseReference(u.Reference).Path)
		}
	}
	for _, a := range result.Aliases {
		if targets[a.Target] {
			addConsumer(ParseReference(a.Reference).Path)
		}
	}
	return consumers
}

// targetsUnderPackageDir collects reference strings that identify code under
// sourceDirectory: path atoms/files and provider import paths for that package tree.
func targetsUnderPackageDir(result *project.Result, rootDir, sourceDirectory string) map[string]bool {
	sourceDirectory = CleanRelDir(sourceDirectory)
	targets := map[string]bool{}
	if sourceDirectory == "" || result == nil {
		return targets
	}
	consider := func(s string) {
		if s == "" || targets[s] {
			return
		}
		if targetRefersToPackageTree(rootDir, ParseReference(s), sourceDirectory) {
			targets[s] = true
		}
	}
	for _, a := range result.Atoms {
		ref := ParseReference(a.Reference)
		p := CleanRelDir(ref.Path)
		if !isUnderDirectory(p, sourceDirectory) {
			continue
		}
		targets[a.Reference] = true
		targets[FileRef("./"+p)] = true
	}
	for _, u := range result.Uses {
		consider(u.Target)
	}
	for _, al := range result.Aliases {
		consider(al.Target)
	}
	return targets
}

// targetRefersToPackageTree reports whether ref names a file/symbol under
// project-relative package directory sourceDirectory (any depth), or a language-provider
// import path for that tree (via PackageImportMatcher — e.g. Go module paths).
func targetRefersToPackageTree(rootDir string, ref Reference, sourceDirectory string) bool {
	sourceDirectory = CleanRelDir(sourceDirectory)
	if sourceDirectory == "" {
		return false
	}
	if ref.Provider == "path" || ref.Provider == "" {
		return isUnderDirectory(CleanRelDir(ref.Path), sourceDirectory)
	}
	return packageImportUnderTree(rootDir, ref.Path, sourceDirectory)
}

// packageImportUnderTree asks registered PackageImportMatchers whether
// importPath refers to packageDir or a subpackage.
func packageImportUnderTree(rootDir, importPath, packageDir string) bool {
	return goImportPathUnderTree(rootDir, importPath, packageDir)
}

func packageImportIsPackage(rootDir, importPath, packageDir string) bool {
	return goImportPathIsPackage(rootDir, importPath, packageDir)
}
