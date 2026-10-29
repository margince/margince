// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The preparation revision of a meeting: the facts an attendee confirms they
// are ready for. A brief written about the meeting is not one of them, so
// regenerating its text never asks anybody to confirm again.

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PreparationFact names one fact a confirmation was given against.
type PreparationFact string

// The facts, as the contract's `changed` list spells them.
const (
	FactStartTime    PreparationFact = "start_time"
	FactAgenda       PreparationFact = "agenda"
	FactParticipants PreparationFact = "participants"
)

// PreparationFacts is what a revision is computed from. The agenda and the
// participants travel as fingerprints, so the stored row keeps no copy of the
// meeting's text or of who sat in it. The agenda is the calendar invite's own
// description, so notes typed into the meeting here never reset readiness.
type PreparationFacts struct {
	StartsAt     time.Time
	StartsOnDay  bool
	Agenda       string
	Participants string
}

// MeetingPreparation is one attendee's preparation state for one meeting.
// Changed is set only when a confirmation exists at an older revision.
type MeetingPreparation struct {
	ActivityID ids.UUID
	Revision   string
	Ready      bool
	Changed    []PreparationFact
}

// preparationFactsOf derives the facts from what the meeting row holds. agenda
// is the fingerprint capture stored, empty for a meeting with no invite. Each
// participant is named by its identity, so the order rows come back in changes
// nothing.
func preparationFactsOf(startsAt time.Time, precision, agenda string, participants []string) PreparationFacts {
	people := slices.Clone(participants)
	slices.Sort(people)
	people = slices.Compact(people)
	return PreparationFacts{
		StartsAt:     startsAt.UTC().Truncate(time.Second),
		StartsOnDay:  precision == precisionDay,
		Agenda:       agenda,
		Participants: fingerprint(people...),
	}
}

// precisionDay is the stored spelling of a meeting with a date and no hour.
const precisionDay = "day"

// Revision is the one preparation revision every reader and the writer use.
func (f PreparationFacts) Revision() string {
	day := "instant"
	if f.StartsOnDay {
		day = precisionDay
	}
	return fingerprint("v1", f.StartsAt.Format(time.RFC3339), day, f.Agenda, f.Participants)
}

// changedSince lists the facts that differ from the ones a confirmation stored.
func (f PreparationFacts) changedSince(confirmed PreparationFacts) []PreparationFact {
	changed := []PreparationFact{}
	if !f.StartsAt.Equal(confirmed.StartsAt) || f.StartsOnDay != confirmed.StartsOnDay {
		changed = append(changed, FactStartTime)
	}
	if f.Agenda != confirmed.Agenda {
		changed = append(changed, FactAgenda)
	}
	if f.Participants != confirmed.Participants {
		changed = append(changed, FactParticipants)
	}
	return changed
}

// preparationAt is the state a reader sees: ready only when the stored
// confirmation is for the current revision.
func preparationAt(id ids.UUID, current PreparationFacts, confirmed *storedReadiness) MeetingPreparation {
	out := MeetingPreparation{ActivityID: id, Revision: current.Revision()}
	if confirmed == nil {
		return out
	}
	if confirmed.revision == out.Revision {
		out.Ready = true
		return out
	}
	out.Changed = current.changedSince(confirmed.facts)
	return out
}

// storedReadiness is a confirmation as the table holds it.
type storedReadiness struct {
	revision string
	facts    PreparationFacts
}

// fingerprint hashes its parts with a separator no part can forge.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}
