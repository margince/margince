// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Naming the record in words.
//
// Somebody says "prep me for the Contoso meeting". The anchor tools took a uuid
// and nothing else, so a host had to call search_records first and thread the id
// through — two calls for one intent, and a lookup that picks the wrong record
// when the name is ambiguous makes the briefing that follows confidently wrong
// about the wrong company.
//
// So the anchor accepts either, and the ambiguity is answered rather than
// guessed. One match is used, and none is refused as none. Several are asked
// about in the chat where the host can show a form (anchorpick.go), and refused
// with the candidates named where it cannot. A refusal a host can act on beats a
// briefing nobody can tell is about the wrong record.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// anchorResolveLimit bounds the candidate read.
//
// Small on purpose: this is a disambiguation, not a search. A name matching more
// than a handful is a name the caller has to narrow however many are listed, and
// a long list in a refusal is one an agent will scan rather than act on.
const anchorResolveLimit = 5

// resolveAnchor answers the record id an anchor names, by id or by name.
//
// Exactly one of the two is supplied — anchorArgs.validate settles that before
// this runs — so this never has to decide which the caller meant.
//
// The name is ranked by the engine search_records runs, scoped to the anchor's
// one type. The two never disagree about which record a word names. Rank order
// picks nothing: one hit is the record, and more are refused.
func resolveAnchor(
	ctx context.Context, search RecordSearch, records datasource.SystemOfRecordProvider, args anchorArgs,
) (ids.UUID, error) {
	if args.RecordID != (ids.UUID{}) {
		return args.RecordID, nil
	}
	// The engine drops a type the caller may not read, which would read as
	// "nothing matches".
	if err := auth.Require(ctx, args.RecordType, principal.ActionRead); err != nil {
		return ids.UUID{}, err
	}
	page, err := search(ctx, RecordSearchQuery{
		Text: args.RecordName, Types: []string{args.RecordType}, Limit: anchorResolveLimit,
	})
	if err != nil {
		return ids.UUID{}, err
	}
	switch len(page.Hits) {
	case 0:
		return ids.UUID{}, &BadArgsError{Cause: fmt.Errorf(
			"no %s matches `record_name` %q — search_records ranks the same words the same way, so try other words there",
			args.RecordType, args.RecordName)}
	case 1:
		return page.Hits[0].Ref.ID, nil
	default:
		if picked, err := pickAnchor(ctx, records, args, page.Hits); err != nil || !picked.IsZero() {
			return picked, err
		}
		return ids.UUID{}, &BadArgsError{Cause: fmt.Errorf(
			"`record_name` %q matches %d %s records — name one by `record_id`: %s",
			args.RecordName, len(page.Hits), args.RecordType, candidateIDs(page.Hits))}
	}
}

// candidateIDs lists the ids a caller may choose between.
//
// IDs and nothing else. The refusal travels to an agent that may hold no grant
// on any of these records, and a name or a field would publish what the record
// says to somebody who asked only whether a word was ambiguous. An id is what
// the caller needs to make the follow-up call, and read_record already gates
// what they may then see.
func candidateIDs(hits []RecordSearchHit) string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.Ref.ID.String())
	}
	return strings.Join(out, ", ")
}
