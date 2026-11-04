// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// renormalizeTranscriptPatch normalizes a body PATCH on a transcript-marked
// row, as ingest does. The transcript retention selector reads that row as
// canonical text, so a patch may not leave raw line endings in it.
func renormalizeTranscriptPatch(current crmcontracts.Activity, in *UpdateActivityInput) error {
	if in.Body == nil || current.SourceSystem == nil || *current.SourceSystem != transcriptSourceSystem {
		return nil
	}
	normalized, err := normalizeTranscript(*in.Body)
	if err != nil {
		return err
	}
	in.Body = &normalized
	return nil
}
