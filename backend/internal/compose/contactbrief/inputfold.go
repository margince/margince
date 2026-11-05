// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func foldClaims(in *Input, view crmcontracts.Contact360) {
	if view.Claims == nil {
		return
	}
	for _, claim := range *view.Claims {
		if len(in.Claims) == briefInputClaims {
			break
		}
		if claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			// A dismissed claim is one a human said was never true. Writing a
			// brief from it would resurrect it in prose.
			continue
		}
		folded := ClaimIn{
			Kind:     string(claim.Kind),
			Body:     claim.Body,
			Status:   string(claim.Status),
			Quote:    claim.SourceQuote,
			SourceID: claim.SourceActivityId.String(),
		}
		if claim.DueAt != nil {
			folded.DueAt = stampAt(*claim.DueAt)
		}
		in.Claims = append(in.Claims, folded)
	}
}

func foldChanges(in *Input, view crmcontracts.Contact360) {
	if view.RelationshipChanges == nil {
		return
	}
	for _, change := range *view.RelationshipChanges {
		if len(in.Changes) == briefInputChanges {
			break
		}
		folded := ChangeIn{Kind: string(change.Kind), At: stampAt(change.At)}
		if change.Days != nil {
			folded.Days = *change.Days
		}
		if change.FromBucket != nil {
			folded.From = string(*change.FromBucket)
		}
		if change.ToBucket != nil {
			folded.To = string(*change.ToBucket)
		}
		in.Changes = append(in.Changes, folded)
	}
}
