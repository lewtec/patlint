package pattern

import (
	"fmt"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
)

// refEmitText turns a product ref into source-like selector text.
// go:context::Background → context.Background
// go:net/http::ListenAndServe → http.ListenAndServe
func refEmitText(ref string) (string, error) {
	ref = strings.TrimPrefix(ref, "@")
	if !strings.Contains(ref, "::") {
		if i := strings.LastIndex(ref, "."); i > 0 {
			if j := strings.Index(ref, ":"); j >= 0 && j < i {
				ref = ref[:i] + "::" + ref[i+1:]
			}
		}
	}
	r := ingest.ParseReference(ref)
	if r.Name == "" {
		if r.Path == "" {
			return "", fmt.Errorf("%w: replacement ref %q: empty symbol", ErrRule, ref)
		}
		return r.Path, nil
	}
	pkg := r.Path
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		pkg = pkg[i+1:]
	}
	if pkg == "" {
		return r.Name, nil
	}
	return pkg + "." + r.Name, nil
}
