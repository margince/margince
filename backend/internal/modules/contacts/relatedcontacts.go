// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// contactRelatedClause narrows the list to the contacts around one contact: its
// current colleagues and the others seated beside it on a deal.
//
// Every hop is the caller's to see. The anchor must be in their contact scope,
// or the filter would say who sits next to a record they cannot open.
func contactRelatedClause(ctx context.Context, anchor *ids.ContactID, arg func(any) int) (string, error) {
	if anchor == nil {
		return "", nil
	}
	anchorPos := arg(*anchor)
	anchorScope, err := scopeOrAllRows(ctx, "contact", "anchor", arg)
	if err != nil {
		return "", err
	}
	colleagues, err := anchorColleaguesSQL(ctx, anchorPos, arg)
	if err != nil {
		return "", err
	}
	related := "contact.id IN (" + colleagues + ")"
	dealPeers, err := anchorDealPeersSQL(ctx, anchorPos, arg)
	if err != nil {
		return "", err
	}
	if dealPeers != "" {
		related += " OR contact.id IN (" + dealPeers + ")"
	}
	return storekit.SQLf(`contact.id <> $%d
		AND EXISTS (SELECT 1 FROM contact anchor
		             WHERE anchor.id = $%d AND anchor.archived_at IS NULL AND (`, anchorPos, anchorPos) +
		anchorScope + `))
		AND (` + related + `)`, nil
}

// anchorColleaguesSQL is the roster of every company the anchor works at now,
// read through the same roster the company page and company_id filter use.
func anchorColleaguesSQL(ctx context.Context, anchorPos int, arg func(any) int) (string, error) {
	edgeBound, err := auth.EdgeReadScope(ctx, "mine", arg)
	if err != nil {
		return "", err
	}
	employers := storekit.SQLf(`ANY (SELECT mine.company_id FROM relationship mine
		 WHERE mine.contact_id = $%d AND `, anchorPos) +
		employment.CurrentMembershipSQL("mine") + ` AND (` + allRowsIfEmpty(edgeBound) + `))`
	return CompanyRosterSQL(ctx, employers, arg)
}

// anchorDealPeersSQL selects the contacts holding a current seat on a deal the anchor
// holds a current seat on. A caller who may not read deals gets no clause, so
// the colleagues alone answer rather than the whole list refusing.
func anchorDealPeersSQL(ctx context.Context, anchorPos int, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	mineBound, err := auth.EdgeReadScope(ctx, "mine", arg)
	if err != nil {
		return "", err
	}
	peerBound, err := auth.EdgeReadScope(ctx, "peer", arg)
	if err != nil {
		return "", err
	}
	dealScope, err := scopeOrAllRows(ctx, "deal", "d", arg)
	if err != nil {
		return "", err
	}
	return storekit.SQLf(`SELECT peer.contact_id
		 FROM relationship mine
		 JOIN deal d ON d.id = mine.deal_id AND d.archived_at IS NULL
		 JOIN relationship peer ON peer.deal_id = mine.deal_id
		  AND peer.kind = '`+dealStakeholderKind+`'
		  AND peer.archived_at IS NULL AND peer.ended_at IS NULL
		 WHERE mine.contact_id = $%d AND mine.kind = '`+dealStakeholderKind+`'
		   AND mine.archived_at IS NULL AND mine.ended_at IS NULL`, anchorPos) +
		` AND (` + allRowsIfEmpty(mineBound) + `) AND (` + allRowsIfEmpty(peerBound) + `) AND (` + dealScope + `)`, nil
}

// allRowsIfEmpty reads an empty scope clause as every row.
func allRowsIfEmpty(clause string) string {
	if clause == "" {
		return scopeAllRows
	}
	return clause
}
