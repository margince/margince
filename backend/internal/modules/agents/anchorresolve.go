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
// guessed: one match is used, several are REFUSED with the candidates named, and
// none is refused as none. Guessing is the outcome this exists to remove — the
// failure it replaces was a silent wrong answer, and a refusal a host can act on
// is strictly better than a briefing nobody can tell is about the wrong record.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
// The search rides the same provider and the same query search_records uses, so
// a name resolves here to exactly what that tool would have returned. Resolving
// through a second reader would let the two disagree about which record a word
// names, which is the confusion this change exists to remove rather than move.
func resolveAnchor(
	ctx context.Context, p datasource.SystemOfRecordProvider, args anchorArgs,
) (ids.UUID, error) {
	if args.RecordID != (ids.UUID{}) {
		return args.RecordID, nil
	}
	res, err := p.Search(ctx, datasource.SearchQuery{
		Text:        args.RecordName,
		EntityTypes: []datasource.EntityType{datasource.EntityType(args.RecordType)},
		Limit:       anchorResolveLimit,
	})
	if err != nil {
		return ids.UUID{}, err
	}
	switch len(res.Records) {
	case 0:
		return ids.UUID{}, &BadArgsError{Cause: fmt.Errorf(
			"no %s matches `record_name` %q — search_records finds what this workspace holds",
			args.RecordType, args.RecordName)}
	case 1:
		return res.Records[0].Ref.ID, nil
	default:
		return ids.UUID{}, &BadArgsError{Cause: fmt.Errorf(
			"`record_name` %q matches %d %s records — name one by `record_id`: %s",
			args.RecordName, len(res.Records), args.RecordType, candidateIDs(res.Records))}
	}
}

// candidateIDs lists the ids a caller may choose between.
//
// IDs and nothing else. The refusal travels to an agent that may hold no grant
// on any of these records, and a name or a field would publish what the record
// says to somebody who asked only whether a word was ambiguous. An id is what
// the caller needs to make the follow-up call, and read_record already gates
// what they may then see.
func candidateIDs(records []datasource.Record) string {
	out := make([]string, 0, len(records))
	for _, rec := range records {
		out = append(out, rec.Ref.ID.String())
	}
	return strings.Join(out, ", ")
}
