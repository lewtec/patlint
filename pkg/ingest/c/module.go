package c

import (
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
)

func init() {
	ingest.RegisterReferenceProvider("c", referenceProvider{})
}

// referenceProvider resolves c:… system-header style specs (no filesystem crawl).
type referenceProvider struct{}

func (referenceProvider) Name() string { return "c" }

func (referenceProvider) Resolve(spec string, _ ingest.ImportResolveContext) (string, bool) {
	spec = strings.TrimSpace(spec)
	spec = strings.TrimPrefix(spec, "<")
	spec = strings.TrimSuffix(spec, ">")
	spec = strings.Trim(spec, `"'`)
	if spec == "" {
		return "", false
	}
	// Path-like relative includes are handled by ResolveInclude, not this provider.
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") || strings.HasPrefix(spec, "/") {
		return "", false
	}
	return "c:" + spec, true
}
