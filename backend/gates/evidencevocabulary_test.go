// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"fmt"
	"regexp"
	"strings"
)

// evidenceVocabularies maps each field_provenance CHECK whose list is the whole
// vocabulary of one evidence view's column to that column. The views store
// nothing, so the vocabulary gates read these lists in their place.
//
// gatekit:fixture each constraint's view column, read from the catalog and
// failing when the constraint stops stating its list
var evidenceVocabularies = map[string]string{
	"field_provenance_company_profile_field":  "company_profile_field.field",
	"field_provenance_company_profile_source": "company_profile_field.source",
	"field_provenance_company_fact_source":    "company_fact.source",
	"field_provenance_contact_profile_field":  "contact_profile_field.field",
}

// evidenceVocabularyCheck reads such a CHECK from the catalog: arms that leave
// other rows alone, then the list as its last arm.
var evidenceVocabularyCheck = regexp.MustCompile(
	`^public\.field_provenance\.([a-z_]+) CHECK \(\(\(value IS NULL\) OR .* OR \([a-z_]+ = ANY \(ARRAY\[([^\]]*)\]\)\)\)\)(?: NOT VALID)?$`)

// readEvidenceVocabularies answers each mapped view column's values. A mapped
// CHECK the catalog no longer states in that shape fails, so the vocabulary
// cannot drop out of the census unseen.
func readEvidenceVocabularies() (map[string][]string, error) {
	raw, err := headCatalogText()
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for line := range strings.SplitSeq(raw, "\n") {
		m := evidenceVocabularyCheck.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		column, mapped := evidenceVocabularies[m[1]]
		if !mapped {
			continue
		}
		var values []string
		for _, lit := range catalogTextLiteral.FindAllStringSubmatch(m[2], -1) {
			values = append(values, strings.ReplaceAll(lit[1], "''", "'"))
		}
		out[column] = asSet(values)
	}
	for constraint, column := range evidenceVocabularies {
		if len(out[column]) == 0 {
			return nil, fmt.Errorf("%s states no list for %s, which holds the vocabulary of %s",
				headCatalogPath, constraint, column)
		}
	}
	return out, nil
}

// isEvidenceVocabularyLine reports whether a catalog line is one of the mapped
// CHECKs, which the census reads through readEvidenceVocabularies.
func isEvidenceVocabularyLine(line string) bool {
	m := evidenceVocabularyCheck.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	_, mapped := evidenceVocabularies[m[1]]
	return mapped
}
