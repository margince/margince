// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The subject-label pass: every card that names a record gets that record's
// display name filled in server-side, under the READER's own grants. Without
// it each client drew N cards and made N follow-up reads to put names on
// them — the N+1 this feed exists to avoid — or, worse, printed ids.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Names resolves one record's display name under the caller's own scope.
//
// ok=false is the REFUSAL answer — the record is gone, archived, or this
// reader may not see it — and the label is then simply absent, which is what
// the contract promises ("Absent when the caller may not read it"). The id
// still travels: existence of the reference is the producer's claim, not
// this resolver's to retract. Any other error propagates; a database that
// will not answer must not read as a record the reader lacks grants for.
//
// Optional exactly as the optional lanes are: nil means this feed sends
// subjects unnamed and the client resolves them itself, which was the
// contract before this seam existed.
type Names interface {
	Label(ctx context.Context, entityType string, id ids.UUID) (label string, ok bool, err error)
}

// fillSubjectLabels names every subject the lanes produced, one gated read
// per DISTINCT record — the same subject on three cards costs one read.
//
// It walks the assembled answer rather than each renderer, so a producer
// added tomorrow gets its labels by existing. The lanes are enumerated from
// the contract struct; a new lane must be added here, and the wiring test
// that asserts every lane's labels is what notices one that was not.
//
// The cost is bounded by the lanes' own caps, and honestly stated: on a
// feed whose every lane is full it is up to ~200 sequential single-row
// gated gets — the at-risk lane alone can admit a hundred candidates. The
// batched resolver (one query per subject TYPE) is the named follow-up
// that removes that; the cache below already collapses duplicates, and
// today's real feeds sit far below the ceiling.
//
// A non-refusal error PROPAGATES and fails the read, deliberately: the
// contract says an absent label means "the caller may not read it", and
// rendering a database failure as absence would tell the reader a record
// was hidden from their account when nothing of the kind is true — the
// same lie face() (render.go) refuses for a merge card's sides.
func (s *Service) fillSubjectLabels(ctx context.Context, out *crmcontracts.Attention) error {
	if s.names == nil {
		return nil
	}
	lanes := []*[]crmcontracts.AttentionItem{
		&out.ThisMorning, &out.NeedsYou, &out.Planned, &out.DoneForYou,
	}
	for _, optional := range []*[]crmcontracts.AttentionItem{
		out.Commitments, out.AtRisk, out.Meetings, out.RelationshipDecay, out.DidNotRun, out.Dsr,
	} {
		if optional != nil {
			lanes = append(lanes, optional)
		}
	}
	type subjectKey struct {
		kind crmcontracts.AttentionSubjectType
		id   ids.UUID
	}
	resolved := map[subjectKey]*string{}
	for _, lane := range lanes {
		for i := range *lane {
			subject := (*lane)[i].Subject
			if subject == nil || subject.Label != nil {
				continue
			}
			key := subjectKey{kind: subject.Type, id: ids.UUID(subject.Id)}
			label, seen := resolved[key]
			if !seen {
				name, ok, err := s.names.Label(ctx, string(subject.Type), ids.UUID(subject.Id))
				if err != nil {
					return err
				}
				if ok {
					label = &name
				}
				resolved[key] = label
			}
			subject.Label = label
		}
	}
	return nil
}
