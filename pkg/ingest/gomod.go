package ingest

import (
	"os"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
)

func goImportPathUnderTree(rootDir, importPath, packageDir string) bool {
	packageDir = CleanRelDir(packageDir)
	p := strings.Trim(strings.TrimPrefix(importPath, "./"), "/")
	if packageDir == "" || p == "" {
		return false
	}
	if mod := readGoModulePath(rootDir); mod != "" {
		prefix := strings.Trim(mod, "/") + "/" + packageDir
		return p == prefix || strings.HasPrefix(p, prefix+"/")
	}
	return p == packageDir || strings.HasPrefix(p, packageDir+"/") || strings.HasSuffix(p, "/"+packageDir)
}

func goImportPathIsPackage(rootDir, importPath, packageDir string) bool {
	packageDir = CleanRelDir(packageDir)
	p := strings.Trim(strings.TrimPrefix(importPath, "./"), "/")
	if p == "" {
		return false
	}
	if mod := readGoModulePath(rootDir); mod != "" {
		want := strings.Trim(mod, "/")
		if packageDir != "" {
			want = want + "/" + packageDir
		}
		return p == want
	}
	if packageDir == "" {
		return false
	}
	return p == packageDir
}

func readGoModulePath(rootDir string) string {
	if rootDir == "" {
		return ""
	}
	data, err := os.ReadFile(lewpath.New(rootDir, "go.mod").String())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
