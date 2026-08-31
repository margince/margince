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
	// Labels answers the display names of one TYPE's records, keyed by id.
	//
	// A record the caller may not read — gone, archived, or out of their row
	// scope — is simply absent from the answer, which is the same refusal the
	// single-record form used to report as ok=false. Absence is not an error:
	// the contract says an unnamed subject means "the caller may not read
	// it", and the id still travels because existence of the reference is the
	// producer's claim, not this resolver's to retract.
	//
	// One call per type, not per record: an implementation is expected to ask
	// its store once. That is the whole reason this seam is shaped by type.
	Labels(ctx context.Context, entityType string, ids []ids.UUID) (map[ids.UUID]string, error)
}

// fillSubjectLabels names every subject the lanes produced, one gated read
// per subject TYPE — every person on the page costs one query, not one each.
//
// It walks the assembled answer rather than each renderer, so a producer
// added tomorrow gets its labels by existing. The lanes are enumerated from
// the contract struct; a new lane must be added here, and the wiring test
// that asserts every lane's labels is what notices one that was not.
//
// The cost is bounded by the number of subject TYPES the page carries —
// six, today — rather than by how full the lanes are. It used to be one
// gated single-row get per distinct record, which on a feed whose every
// lane is full is around two hundred sequential reads: cheap individually,
// linear in exactly the thing a busy workspace has more of.
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
		out.Commitments, out.AtRisk, out.Meetings, out.RelationshipDecay, out.DidNotRun, out.Dsr, out.SyncHealth, out.CaptureHealth, out.AiWorkHealth, out.Bounces, out.AutomationHealth, out.Notices,
	} {
		if optional != nil {
			lanes = append(lanes, optional)
		}
	}
	// Gathered before anything is asked, so each type is one question. The
	// ids are DEDUPED per type — the same person on three cards is one id in
	// the query — and the order they were met in is kept, so a store that
	// bounds its answer drops the same records for the same page rather than
	// a different set each read.
	wanted := map[crmcontracts.AttentionSubjectType][]ids.UUID{}
	seen := map[crmcontracts.AttentionSubjectType]map[ids.UUID]bool{}
	for _, lane := range lanes {
		for i := range *lane {
			subject := (*lane)[i].Subject
			if subject == nil || subject.Label != nil {
				continue
			}
			id := ids.UUID(subject.Id)
			if seen[subject.Type] == nil {
				seen[subject.Type] = map[ids.UUID]bool{}
			}
			if seen[subject.Type][id] {
				continue
			}
			seen[subject.Type][id] = true
			wanted[subject.Type] = append(wanted[subject.Type], id)
		}
	}

	resolved := map[crmcontracts.AttentionSubjectType]map[ids.UUID]string{}
	for kind, batch := range wanted {
		labels, err := s.names.Labels(ctx, string(kind), batch)
		if err != nil {
			return err
		}
		resolved[kind] = labels
	}

	for _, lane := range lanes {
		for i := range *lane {
			subject := (*lane)[i].Subject
			if subject == nil || subject.Label != nil {
				continue
			}
			if label, ok := resolved[subject.Type][ids.UUID(subject.Id)]; ok {
				name := label
				subject.Label = &name
			}
		}
	}
	return nil
}
