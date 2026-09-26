// Package jvm registers the JVM language family (package-directory module lattice).
// Surfaces (java, scala, kotlin, …) blank-import this package and join via Family.
package jvm

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/project"
	javaref "github.com/lewtec/patlint/pkg/reference/java"
	kotlinref "github.com/lewtec/patlint/pkg/reference/kotlin"
	scalaref "github.com/lewtec/patlint/pkg/reference/scala"
)

// FamilyID is the registry id for the JVM lattice.
const FamilyID = "jvm"

// Family is the JVM family handle. Surfaces join via pack as-family.
var Family = ingest.RegisterFamily(FamilyID, ingest.FamilySpec{
	Lattice:       lattice{},
	ResolveImport: resolveJVMImport,
})

func resolveJVMImport(spec string, ctx ingest.ImportResolveContext) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	hits := []string{
		javaref.ResolveImport(spec, ctx.KnownFiles),
		kotlinref.ResolveImport(spec, ctx.KnownFiles),
		scalaref.ResolveImport(spec, ctx.KnownFiles),
	}
	for _, r := range hits {
		if strings.HasPrefix(r, "path:") {
			return r
		}
	}
	imp := strings.TrimPrefix(ctx.ImporterPath, "./")
	switch {
	case strings.HasSuffix(imp, ".kt"), strings.HasSuffix(imp, ".kts"):
		return "kotlin:" + spec
	case strings.HasSuffix(imp, ".scala"):
		return "scala:" + spec
	default:
		return "java:" + spec
	}
}

type lattice struct{}

func (lattice) Grains() []ingest.MoveGrain {
	return []ingest.MoveGrain{ingest.MoveGrainAtom, ingest.MoveGrainPackage}
}

func (lattice) ModuleKey(filePath string) string { return ingest.DirModuleKey(filePath) }

func (m lattice) SameModule(a, b string) bool { return m.ModuleKey(a) == m.ModuleKey(b) }

func (lattice) ListNodes(result *project.Result, grain ingest.MoveGrain, projectFamily string) []ingest.MoveNode {
	switch grain {
	case ingest.MoveGrainAtom:
		return ingest.ListAtomMoveNodes(result, projectFamily, nil)
	case ingest.MoveGrainPackage:
		return filterOutJVMSourceRoots(ingest.ListPackageMoveNodes(result, projectFamily, nil))
	default:
		return nil
	}
}

var jvmSourceRootSuffixes = []string{
	"src/main/java-templates",
	"src/test/java-templates",
	// KMP source sets (kotlinpoet, okio, …) — longer than bare src/main/kotlin.
	"src/commonMain/kotlin",
	"src/commonTest/kotlin",
	"src/jvmMain/kotlin",
	"src/jvmTest/kotlin",
	"src/jsMain/kotlin",
	"src/jsTest/kotlin",
	"src/wasmJsMain/kotlin",
	"src/wasmJsTest/kotlin",
	"src/nativeMain/kotlin",
	"src/nativeTest/kotlin",
	"src/androidMain/kotlin",
	"src/androidUnitTest/kotlin",
	"src/main/java",
	"src/test/java",
	"src/main/scala",
	"src/test/scala",
	"src/main/kotlin",
	"src/test/kotlin",
	"src/jmh/java",
	"src/jmh/scala",
	"src/jmh/kotlin",
	"src/testFixtures/java",
	"src/testFixtures/scala",
	"src/testFixtures/kotlin",
	"src/integrationTest/java",
	"src/integrationTest/scala",
	"src/integrationTest/kotlin",
	"src/androidTest/java",
	"src/androidTest/kotlin",
	"src/main/resources",
	"src/test/resources",
	"src",
}

func IsSourceRootDir(dir string) bool {
	dir = strings.Trim(strings.TrimPrefix(dir, "./"), "/")
	if dir == "" {
		return false
	}
	for _, root := range jvmSourceRootSuffixes {
		if dir == root || strings.HasSuffix(dir, "/"+root) {
			return true
		}
	}
	return false
}

func IsTemplatePackageDir(dir string) bool {
	dir = strings.Trim(strings.TrimPrefix(dir, "./"), "/")
	if dir == "" {
		return false
	}
	return strings.Contains(dir, "java-templates/") ||
		strings.HasSuffix(dir, "java-templates")
}

func filterOutJVMSourceRoots(nodes []ingest.MoveNode) []ingest.MoveNode {
	var out []ingest.MoveNode
	for _, n := range nodes {
		if IsSourceRootDir(n.Path) || IsTemplatePackageDir(n.Path) {
			continue
		}
		// also check without ./
		rel := strings.TrimPrefix(n.Path, "./")
		if IsSourceRootDir(rel) || IsTemplatePackageDir(rel) {
			continue
		}
		out = append(out, n)
	}
	return out
}
