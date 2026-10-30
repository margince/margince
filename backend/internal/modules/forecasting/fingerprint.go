// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// InputFingerprint is a digest of what a set of contributions says about the
// forecast. Two readings with one fingerprint agree on every deal's money,
// close-window membership and qualification. No comparison between them can
// find a material change. The owner and audit references move no figure.
func InputFingerprint(rows []Contribution) string {
	ordered := slices.SortedFunc(slices.Values(rows), func(a, b Contribution) int {
		return strings.Compare(a.DealID, b.DealID)
	})
	var material []byte
	for _, row := range ordered {
		base := "-"
		if row.BaseMinor != nil {
			base = fmt.Sprint(*row.BaseMinor)
		}
		material = fmt.Appendf(material, "%s|%s|%d|%t|%t|%t|%t|%t|%s|%s|%s\n",
			row.DealID, base, row.WeightedMinor, row.InWon, row.InOpen, row.InEvidence,
			row.InBestCase, row.InLost, row.Qualification, row.Category, row.ExclusionReason)
	}
	digest := sha256.Sum256(material)
	return hex.EncodeToString(digest[:])
}
