// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A create that filed a duplicate for review says so in its own answer.
//
// WHY THIS EXISTS. The ladder behind create_record already detects duplicates
// and files them on the review queue — an exact phone collision, a fuzzy
// near-match, two records written with the same name. It did all of that
// silently. The tool answered with the created record and nothing else, so an
// assistant that had just filed a candidate could not see it, and told its user
// the opposite: asked whether a same-name same-phone create was caught, it
// reported "the guard is email-only" while a candidate row sat in the queue at
// confidence 1.000.
//
// That is worse than a missing feature. A user reading the chat is told a
// working safeguard does not exist, which argues for building something that
// is already there and undermines trust in the guards that do fire.
//
// WHY IT IS READ BACK RATHER THAN RETURNED BY THE WRITE. The provider interface
// is FROZEN at v1 (datasource.SystemOfRecordProvider): a fork may ship its own
// adapter, so widening Create's return would break every one of them. The
// candidate is read after the write instead, through a seam compose injects —
// the same shape every other read on this surface takes, so RBAC and row scope
// apply to it exactly as they do on the HTTP path.
//
// WHY IT DOES NOT FAIL THE CALL. The record is already written and committed by
// the time this runs. A read that fails here must not turn a successful create
// into an error the caller will retry — a retry would create a SECOND duplicate,
// which is precisely the harm the report exists to prevent. So the failure is
// carried as a warning: the caller learns the check could not be made, rather
// than being told there was nothing to find.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// DuplicateCandidate is one open review-queue pair naming a record this call
// just created, in the terms a caller acts on.
type DuplicateCandidate struct {
	// CandidateID names the review-queue pair, which is what dismissing or
	// re-opening it takes.
	CandidateID string `json:"candidate_id"`
	// OtherRecordID is the record already in the workspace — never the one that
	// was just created. A caller offering a merge needs to know which is which,
	// and working it out from a left/right pair is a step it should not have to
	// take.
	OtherRecordID string `json:"other_record_id"`
	// Confidence is the detection's own score, 0..1.
	Confidence float64 `json:"confidence"`
	// Evidence is the detection-time snapshot: the field that collided and the
	// two values, as the matcher saw them. Passed through rather than
	// paraphrased, because what a reviewer needs is what was actually compared.
	Evidence []DuplicateEvidence `json:"evidence"`
}

// DuplicateEvidence is one axis two records met on.
//
// TYPED rather than a free map, and not only because a schema needs a shape:
// this is what a caller reads out to a contact before offering a merge, and the
// members it can rely on ought to be the ones the contract names. The stored
// snapshot is written by this system in exactly these five keys.
type DuplicateEvidence struct {
	// Field is the axis, such as "phone" or "email". Label names it in the
	// installation's language.
	Field string `json:"field"`
	Label string `json:"label,omitempty"`
	// Left is the value on the record this call created and Right the one
	// already here, in their stored form: detection writes the evidence from
	// the created record's side, whichever order the pair's ids sort in.
	//
	// Either may be empty: a one-sided signal is a fact one record carries and
	// the other does not, which is itself evidence.
	Left  string `json:"left_value,omitempty"`
	Right string `json:"right_value,omitempty"`
	// Signal is how they met: "collide" for two values that are the same or
	// alike, "one_sided" for a value only one record holds.
	Signal string `json:"signal,omitempty"`
	// Score is this axis's own contribution, 0..1.
	Score float64 `json:"score,omitempty"`
}

// OpenDuplicatesFor answers the open candidates naming one record, or none.
// A non-nil only keeps just those pair ids.
//
// It is a seam rather than a call into the contacts module because agents may not
// import it (see .go-arch-lint.yml) — and should not: the injected reader is
// what keeps this surface unable to reach a record table on its own.
type OpenDuplicatesFor func(ctx context.Context, recordType string, id ids.UUID, only []ids.UUID) ([]DuplicateCandidate, error)

// CodeDuplicateFiled is the warning a create carries when it filed a pair for
// review, and CodeDuplicateCheckFailed when it could not find out.
//
// They are separate codes because they license different sentences. The first
// says a duplicate WAS found and a human has been asked. The second says nothing
// about whether one exists — and a caller that folded them together would
// report "no duplicate" for a check that never ran, which is the failure this
// whole file is a correction for.
const (
	CodeDuplicateFiled       = "duplicate_filed_for_review"
	CodeDuplicateCheckFailed = "duplicate_check_unavailable"
)

const duplicateCheckFailedMessage = "The record was created. Whether it duplicates one already here could not be checked, " +
	"so treat this as unknown rather than as no duplicate."

// duplicateWarning renders the human-readable half. It names the count and what
// happens next, and deliberately does NOT say the record was rejected or
// merged: it was neither. The record exists, and a human will decide.
func duplicateWarning(n int) Warning {
	subject := "A record already here looks like this one"
	if n > 1 {
		subject = "Records already here look like this one"
	}
	return Warning{
		Code: CodeDuplicateFiled,
		Message: subject + ", so the pair was filed for a human to review. " +
			"The record was still created and nothing was merged — " +
			"read the candidate's evidence before offering to merge them. " +
			"Tell the user the pair is waiting for that review; whether the two are one " +
			"is the reviewer's call, not yours.",
	}
}

// createdRecord is the created record PLUS what the create filed for review.
//
// A type of its own rather than a field on wireRecord, which every read and
// search also rides: a duplicate report is a fact about this WRITE, and hanging
// it on the shared record shape would put a permanently-absent member on every
// read answer on the surface.
//
// The embed keeps the record's own members at the top level, so the shape a
// caller already parses is unchanged and the new member is additive.
type createdRecord struct {
	wireRecord
	// DuplicateCandidates is omitted entirely when the create filed nothing,
	// which is the common case. Present and non-empty means a human has been
	// asked about this record — see the warning that travels with it.
	DuplicateCandidates []DuplicateCandidate `json:"duplicate_candidates,omitempty"`
	// TagOffer is present only when the caller proposed a word with offer_tag
	// and it can be offered. It records an offer; nothing was applied.
	TagOffer *TagOffer `json:"tag_offer,omitempty"`
	// Changes is every field the call set, as the record now holds it. Absent
	// when the fields could not be named, which a warning then says.
	Changes []FieldChange `json:"changes,omitempty"`
}

// employmentEdgeContact names the contact an employment-edge create gives an
// employer to, and reports false for every other create.
func employmentEdgeContact(recordType string, fields json.RawMessage) (ids.UUID, bool) {
	if recordType != string(datasource.EntityRelationship) {
		return ids.UUID{}, false
	}
	var edge struct {
		Kind      string `json:"kind"`
		ContactID string `json:"contact_id"`
	}
	if err := json.Unmarshal(fields, &edge); err != nil || edge.Kind != "employment" {
		return ids.UUID{}, false
	}
	contact, err := ids.Parse(edge.ContactID)
	return contact, err == nil
}

// reportCreateDuplicates is reportDuplicates for any create. An employment edge
// is reported against its contact, limited to the pairs this create filed.
// The write itself noted them in filed, so a concurrent writer's pair on the
// same contact is not claimed.
func (t createRecord) reportCreateDuplicates(ctx context.Context, recordType string, fields json.RawMessage, id ids.UUID, filed []ids.UUID) []DuplicateCandidate {
	contact, edge := employmentEdgeContact(recordType, fields)
	if recordType == string(datasource.EntityRelationship) && !edge {
		return nil
	}
	if !edge {
		return t.reportDuplicates(ctx, recordType, id, nil)
	}
	if len(filed) == 0 {
		return nil
	}
	return t.reportDuplicates(ctx, "contact", contact, filed)
}

// reportDuplicates tells the caller what this create filed for review.
//
// It returns nothing, and that is deliberate: the record is written and
// committed by the time it runs, so no failure here may turn a successful
// create into an error. A caller handed an error retries, and a retry creates a
// SECOND duplicate — the exact harm the report exists to prevent.
//
// So both outcomes travel as warnings, under two codes, because they license
// different sentences. "A pair was filed" says a duplicate was found and a human
// asked. "The check could not run" says nothing about whether one exists —
// folding them together would let a caller report "no duplicate found" for a
// check that never happened, which is the failure this whole path corrects.
func (t createRecord) reportDuplicates(ctx context.Context, recordType string, id ids.UUID, only []ids.UUID) []DuplicateCandidate {
	if t.duplicates == nil {
		return nil
	}
	found, err := t.duplicates(ctx, recordType, id, only)
	if err != nil {
		noteWarning(ctx, CodeDuplicateCheckFailed, duplicateCheckFailedMessage)
		return nil
	}
	if len(found) == 0 {
		return nil
	}
	// Both channels, and each carries what the other cannot. The warning is what
	// a model reads without being asked; the data is what it acts on — the other
	// record's id, which no sentence should make it parse out of prose.
	w := duplicateWarning(len(found))
	noteWarning(ctx, w.Code, w.Message)

	lang := summaryIn(ctx, t.language).lang
	for _, d := range found {
		for i := range d.Evidence {
			d.Evidence[i].Label = FieldLabel(lang, d.Evidence[i].Field)
		}
		// Each disclosed record is cited: naming a record to an agent hands it
		// over, so the evidence list has to say so. Without this a create
		// discloses up to five other records while citing only the one it wrote.
		if other, err := ids.Parse(d.OtherRecordID); err == nil {
			noteEvidence(ctx, datasource.EntityType(recordType), other)
		}
	}
	// The evidence QUOTES the other record — a phone number, a name, an address
	// as it is stored there — and this call never read that row's provenance, so
	// it cannot know what tier the values arrived at. A contact captured off an
	// inbound mail is T2, and carrying its number under the T1 envelope of a
	// record a human just typed would raise the trust of the untrusted half.
	// Derived content is the honest label for material whose provenance this
	// answer did not read.
	noteDerivedContent(ctx)
	return found
}
