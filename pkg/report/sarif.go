package report

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	lewpath "github.com/lewtec/lewkit/x/path"
	"github.com/lewtec/patlint/pkg/project"

	"github.com/lewtec/lewkit/x/release"
	"github.com/lewtec/lewkit/x/text"
)

// WriteSARIF writes a SARIF 2.1.0 log for findings to w.
// Findings with successful site edits always include result.fixes.
// If rules is empty, descriptors are derived from findings.
func WriteSARIF(w io.Writer, root string, findings []Finding, rules []Rule) error {
	if len(rules) == 0 {
		rules = rulesFromFindings(findings)
	}

	outRules := make([]sarifReportingDescriptor, 0, len(rules))
	ruleIndex := map[string]int{}
	for i, r := range rules {
		ruleIndex[r.ID] = i
		outRules = append(outRules, sarifReportingDescriptor{
			ID:               r.ID,
			Name:             r.ID,
			ShortDescription: sarifMessage{Text: r.Message},
			FullDescription:  sarifMessage{Text: r.Message},
			DefaultConfiguration: &sarifReportingConfiguration{
				Level: r.Level.String(),
			},
		})
	}

	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		idx, ok := ruleIndex[f.RuleID]
		if !ok {
			idx = -1
		}
		uri := toSARIFURI(f.File)
		r := sarifResult{
			RuleID:    f.RuleID,
			RuleIndex: idx,
			Level:     f.Level.String(),
			Message:   sarifMessage{Text: f.Message},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: uri},
					Region: sarifRegion{
						StartLine:   f.Line,
						StartColumn: f.Column,
						EndLine:     f.EndLine,
						EndColumn:   f.EndCol,
						Snippet:     &sarifArtifactContent{Text: f.Snippet},
					},
				},
			}},
		}
		if f.Fixable && !f.FixSkipped && len(f.SiteEdits) > 0 {
			fix, err := editsToSARIFFix(root, f.Message, f.SiteEdits, f.Source)
			if err != nil {
				return err
			}
			r.Fixes = []sarifFix{fix}
		}
		results = append(results, r)
	}

	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{
				Driver: sarifToolComponent{
					Name:           "refactree",
					Version:        release.Version(),
					InformationURI: "https://github.com/lewtec/patlint",
					Rules:          outRules,
				},
			},
			Results: results,
		}},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func rulesFromFindings(findings []Finding) []Rule {
	seen := map[string]Rule{}
	order := []string{}
	for _, f := range findings {
		if _, ok := seen[f.RuleID]; ok {
			continue
		}
		seen[f.RuleID] = Rule{ID: f.RuleID, Message: f.Message, Level: f.Level}
		order = append(order, f.RuleID)
	}
	out := make([]Rule, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}

func editsToSARIFFix(root, description string, edits []project.Edit, source []byte) (sarifFix, error) {
	byFile := map[string][]project.Edit{}
	order := []string{}
	for _, e := range edits {
		if _, ok := byFile[e.File]; !ok {
			order = append(order, e.File)
		}
		byFile[e.File] = append(byFile[e.File], e)
	}
	changes := make([]sarifArtifactChange, 0, len(order))
	for _, file := range order {
		fileEdits := byFile[file]
		src := source
		if src == nil {
			abs := file
			if root != "" && !filepath.IsAbs(file) {
				abs = lewpath.New(root, filepath.FromSlash(file)).String()
			}
			b, err := os.ReadFile(abs)
			if err != nil {
				return sarifFix{}, err
			}
			src = b
		}
		li := text.NewLineIndexBytes(src)
		repls := make([]sarifReplacement, 0, len(fileEdits))
		for _, e := range fileEdits {
			sl, sc0 := li.LineColumnAtU32(e.StartByte)
			el, ec0 := li.LineColumnAtU32(e.EndByte)
			repls = append(repls, sarifReplacement{
				DeletedRegion: sarifRegion{
					StartLine:   sl,
					StartColumn: sc0 + 1,
					EndLine:     el,
					EndColumn:   ec0 + 1,
				},
				InsertedContent: &sarifArtifactContent{Text: e.NewText},
			})
		}
		changes = append(changes, sarifArtifactChange{
			ArtifactLocation: sarifArtifactLocation{URI: toSARIFURI(file)},
			Replacements:     repls,
		})
	}
	return sarifFix{
		Description:     &sarifMessage{Text: description},
		ArtifactChanges: changes,
	}, nil
}

func toSARIFURI(file string) string {
	return strings.TrimPrefix(filepath.ToSlash(file), "./")
}

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifToolComponent `json:"driver"`
}

type sarifToolComponent struct {
	Name           string                     `json:"name"`
	Version        string                     `json:"version,omitempty"`
	InformationURI string                     `json:"informationUri,omitempty"`
	Rules          []sarifReportingDescriptor `json:"rules,omitempty"`
}

type sarifReportingDescriptor struct {
	ID                   string                       `json:"id"`
	Name                 string                       `json:"name,omitempty"`
	ShortDescription     sarifMessage                 `json:"shortDescription,omitempty"`
	FullDescription      sarifMessage                 `json:"fullDescription,omitempty"`
	DefaultConfiguration *sarifReportingConfiguration `json:"defaultConfiguration,omitempty"`
}

type sarifReportingConfiguration struct {
	Level string `json:"level,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"` // 0 is valid; do not omitempty
	Level     string          `json:"level,omitempty"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
	Fixes     []sarifFix      `json:"fixes,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int                   `json:"startLine,omitempty"`
	StartColumn int                   `json:"startColumn,omitempty"`
	EndLine     int                   `json:"endLine,omitempty"`
	EndColumn   int                   `json:"endColumn,omitempty"`
	Snippet     *sarifArtifactContent `json:"snippet,omitempty"`
}

type sarifArtifactContent struct {
	Text string `json:"text,omitempty"`
}

type sarifFix struct {
	Description     *sarifMessage         `json:"description,omitempty"`
	ArtifactChanges []sarifArtifactChange `json:"artifactChanges"`
}

type sarifArtifactChange struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Replacements     []sarifReplacement    `json:"replacements"`
}

type sarifReplacement struct {
	DeletedRegion   sarifRegion           `json:"deletedRegion"`
	InsertedContent *sarifArtifactContent `json:"insertedContent,omitempty"`
}
