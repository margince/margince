// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	_ "embed"
	"encoding/json"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

//go:embed previewcatalog_gen.json
var previewCatalogJSON []byte

type previewLanguage struct {
	Outcomes map[string]string `json:"outcomes"`
	Sections map[string]string `json:"sections"`
	Labels   map[string]string `json:"labels"`
	Coded    map[string]string `json:"coded"`
	Defaults map[string]string `json:"defaults"`
}

var previewCatalog = func() map[string]previewLanguage {
	var catalog struct {
		Languages map[string]previewLanguage `json:"languages"`
	}
	if err := json.Unmarshal(previewCatalogJSON, &catalog); err != nil {
		panic(err)
	}
	return catalog.Languages
}()

func readingLanguage(lang textlang.Lang) previewLanguage {
	if catalog, ok := previewCatalog[string(lang)]; ok {
		return catalog
	}
	return previewCatalog["en"]
}

// PreviewFieldLabel uses the card's generated catalog, including its argument labels.
func PreviewFieldLabel(lang textlang.Lang, field string) string {
	if label, ok := strings.CutPrefix(field, "argument:"); ok {
		return label
	}
	if label, ok := readingLanguage(lang).Labels[field]; ok {
		return label
	}
	return FieldLabel(lang, field)
}

func previewCodedValue(lang textlang.Lang, field string, value crmcontracts.ApprovalPreviewValue) crmcontracts.ApprovalPreviewValue {
	catalog := readingLanguage(lang)
	key := field + "/" + value.Kind
	label, ok := catalog.Coded[key+"/"+value.Text]
	if !ok {
		label, ok = catalog.Defaults[key]
	}
	if ok {
		value.Text, value.Kind = label, "text"
	}
	return value
}

// PreviewSectionLabel keeps email section headings aligned with the card.
func PreviewSectionLabel(lang textlang.Lang, section string) string {
	return readingLanguage(lang).Sections[section]
}

func previewImportOutcome(lang textlang.Lang, outcome string) string {
	if label, ok := readingLanguage(lang).Outcomes[outcome]; ok {
		return label
	}
	return outcome
}
