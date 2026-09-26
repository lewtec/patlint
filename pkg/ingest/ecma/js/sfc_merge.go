package js

import (
	"context"
	"github.com/lewtec/patlint/pkg/project"
	"strings"

	"github.com/lewtec/patlint/pkg/ingest"
	"github.com/lewtec/patlint/pkg/ingest/ecma"
	"github.com/lewtec/patlint/pkg/ingestutil"
	"github.com/lewtec/patlint/pkg/sitter"
)

// MergeScriptElement re-parses a script_element raw_text as ECMA and merges
// into fe with host offsets. grammarName is the pack guest id (required).
func MergeScriptElement(ctx context.Context, policy ingest.PackQueries, fe *project.FileExtract, scriptEl *sitter.Node, source []byte, relPath, grammarName string) {
	if fe == nil || scriptEl == nil {
		return
	}
	raw := ingestutil.ChildByType(scriptEl, "raw_text")
	if raw == nil {
		return
	}
	if grammarName == "" {
		return
	}
	MergeRawScript(ctx, policy, fe, raw, source, relPath, grammarName)
}

// MergeRawScript re-parses raw node text as ECMA and merges into fe with offsets.
func MergeRawScript(ctx context.Context, policy ingest.PackQueries, fe *project.FileExtract, raw *sitter.Node, source []byte, relPath, grammarName string) {
	start, end, ok := ecma.RawNodeSpan(raw, source)
	if !ok {
		return
	}
	sub, err := ExtractECMAScript(ctx, policy, source[start:end], grammarName, relPath)
	if err != nil || sub == nil {
		return
	}
	ecma.OffsetMergeExtract(fe, sub, start)
}

// MergeExpressionUsages parses expression text under raw and records usages.
func MergeExpressionUsages(ctx context.Context, policy ingest.PackQueries, fe *project.FileExtract, raw *sitter.Node, source []byte, grammarName string) {
	start, end, ok := ecma.RawNodeSpan(raw, source)
	if !ok {
		return
	}
	expr := source[start:end]
	trim := strings.TrimSpace(string(expr))
	if trim == "" || ecma.IsAllDigits(trim) {
		return
	}
	usages, err := ExtractECMAExpressionUsages(ctx, policy, expr, grammarName)
	if err != nil {
		return
	}
	for _, u := range usages {
		u.StartByte += start
		u.EndByte += start
		for i := range u.Prefix {
			u.Prefix[i].StartByte += start
			u.Prefix[i].EndByte += start
		}
		fe.Usages = append(fe.Usages, u)
	}
}
