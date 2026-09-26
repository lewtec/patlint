package ingest

// Product highlight CSS classes for structural-tape leaves. Stable across languages;
// which grammar node types map to which class is pack as-keyword / as-ident / ….
const (
	HLKeyword = "tok-kw"
	HLString  = "tok-str"
	HLNumber  = "tok-num"
	HLComment = "tok-comment"
	HLIdent   = "tok-ident"
	HLType    = "tok-type"
	HLConst   = "tok-const"
	HLOp      = "tok-op"
	HLPunct   = "tok-punct"
	HLOther   = "tok-other"
)

// ClassifyLeafLanguage maps a leaf type using pack as-keyword / as-ident / …
// paint (including highlight.rft global as-op / as-punct).
func ClassifyLeafLanguage(policy PackQueries, language, grammarType string) string {
	if policy == nil || language == "" || grammarType == "" {
		return ""
	}
	return policy.ClassifyLeaf(language, grammarType)
}
