// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "slices"

// FooterFinding describes a disclosure or private link omitted from one message.
type FooterFinding struct {
	Kind         string   `json:"kind"`
	Language     string   `json:"language"`
	MissingMeans []string `json:"missing_means"`
}

// SameFooterFindings reports whether two lists name the same missing
// disclosures, which is how a stored review is told apart from a changed one.
func SameFooterFindings(a, b []FooterFinding) bool {
	return slices.EqualFunc(a, b, func(x, y FooterFinding) bool {
		return x.Kind == y.Kind && x.Language == y.Language && slices.Equal(x.MissingMeans, y.MissingMeans)
	})
}
