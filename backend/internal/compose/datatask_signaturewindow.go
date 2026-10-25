// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// signatureWindowDataTask takes back signature fields the writer would refuse
// today. Evidence outside signatureBlock came from a quoted or forwarded mail.
// A number contacts.IsMemberNumber names is a colleague's. A field whose mail
// is gone, archived or restricted has no source left to stand on.
func signatureWindowDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792842314_signature_fields_outside_the_senders_text", Version: 1,
		Title: "Taking back contact details read from quoted mail", Requires: []string{"core/1792842046"},
		Steps: []datatask.Step{{
			Name: "signature_fields", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateSignatureFields, Batch: retractOutsideSignatureWindowBatch,
		}},
	}
}

func retractOutsideSignatureWindowBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.SignatureFieldSourcesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(page))
	var quoted, members, unreadable []ids.UUID
	memberNumbers := map[ids.ContactID]map[string]bool{}
	for _, field := range page {
		scanned = append(scanned, field.ID)
		if field.Body == nil {
			unreadable = append(unreadable, field.ID)
			continue
		}
		if field.Field == fieldPhone {
			numbers, err := memberNumbersOf(ctx, tx, field.ContactID, memberNumbers)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if contacts.IsMemberNumber(numbers, field.Value) {
				members = append(members, field.ID)
				continue
			}
		}
		if !evidenceInSignatureWindow(*field.Body, field.Evidence) {
			quoted = append(quoted, field.ID)
		}
	}
	changed := 0
	for _, retraction := range []struct {
		fields []ids.UUID
		reason string
	}{
		{quoted, "signature read off text the contact did not write in that mail"},
		{members, "a number a workspace member signs with"},
		{unreadable, "the mail it was read from is gone, archived or restricted"},
	} {
		removed, err := contacts.RetractSignatureFieldsByIDTx(ctx, tx, retraction.fields, retraction.reason)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		changed += int(removed)
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// memberNumbersOf reads one contact's member numbers once per batch.
func memberNumbersOf(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, seen map[ids.ContactID]map[string]bool) (map[string]bool, error) {
	if numbers, ok := seen[contactID]; ok {
		return numbers, nil
	}
	numbers, err := contacts.MemberNumbersTx(ctx, tx, contactID)
	if err != nil {
		return nil, err
	}
	seen[contactID] = numbers
	return numbers, nil
}

// evidenceInSignatureWindow is the evidence gate's own test (gateEvidence),
// asked of the window the pass would show the model today.
func evidenceInSignatureWindow(body, evidence string) bool {
	window := signatureBlock(body)
	return evidenceOnPage(window, normalizeEvidence(window), evidence)
}

func estimateSignatureFields(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountSignatureFieldSources(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
