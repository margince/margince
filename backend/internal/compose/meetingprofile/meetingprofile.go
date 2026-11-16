// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package meetingprofile is what a transcript reading says about how the
// company on the other side sells to its own customers. It holds the reply's
// part that carries it and the check it must pass.
//
// It rides the reading's one model call. A part that fails the check is
// dropped, so it never fails the reading.
package meetingprofile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/commitmentparty"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// Key is the reply's field for the company profile.
const Key = "company_profile"

// MaxValue bounds one field's value: a profile field is a few sentences.
const MaxValue = 400

// Fields are the profile fields a meeting may fill: how the company sells,
// as its own staff describe it. What it does and its legal identity come
// from its website.
var Fields = []string{
	string(crmcontracts.CompanyProfileFieldFieldIcp),
	string(crmcontracts.CompanyProfileFieldFieldCustomerPains),
	string(crmcontracts.CompanyProfileFieldFieldDesiredOutcomes),
	string(crmcontracts.CompanyProfileFieldFieldBuyingCenter),
	string(crmcontracts.CompanyProfileFieldFieldBuyingIntents),
	string(crmcontracts.CompanyProfileFieldFieldCommonObjections),
	string(crmcontracts.CompanyProfileFieldFieldSalesMotion),
}

// Entry is one field the meeting states.
type Entry struct {
	// SpeakerRef is the verified participant reference of the outside speaker
	// who says it; the company is theirs.
	SpeakerRef  string  `json:"speaker_ref"`
	Field       string  `json:"field"`
	Value       string  `json:"value"`
	SourceLines []int   `json:"source_lines"`
	Confidence  float64 `json:"confidence"`
}

// Instruction asks for the profile, after the reading's JSON instruction has
// named Key among the fields of the reply.
func Instruction() string {
	return fmt.Sprintf(`"%s" lists what the meeting states about how the OTHER company — the customer `+
		`or prospect we met, never us — sells to its OWN customers. Each entry is {"speaker_ref", "field", "value", `+
		`"source_lines", "confidence"}, at most one per field and speaker. "speaker_ref" is the verified `+
		`participant reference of the outside speaker who says it; leave the entry out when no verified `+
		`outside participant says it. "field" is one of: icp (whom it sells to), `+
		`customer_pains (problems that bring its customers to it), desired_outcomes (what its customers want `+
		`to achieve), buying_center (who evaluates, buys or approves when its customers buy), buying_intents `+
		`(what signals that its customers want to buy), common_objections (objections its customers raise), `+
		`sales_motion (how a typical sale of theirs happens). "value" is at most %d characters in the `+
		`transcript's language. Report a field only when a speaker of that company says it; how they buy `+
		`from us, and anything you infer, is not a field. "source_lines" are the 1 to %d line numbers it is `+
		`stated on. An empty list is the common answer.`,
		Key, MaxValue, activities.MaxTranscriptCitedLines)
}

// Schema is the profile's shape in the reply's response schema.
func Schema() schema.Node {
	return schema.Array(schema.Object(map[string]schema.Node{
		"speaker_ref":  schema.String(),
		"field":        schema.String(),
		"value":        schema.String(),
		"source_lines": schema.Array(schema.Integer()),
		"confidence":   schema.Number(),
	}, "speaker_ref", "field", "value", "source_lines", "confidence"))
}

// Read decodes the profile from the reply and checks it against a transcript
// of lineCount lines. A non-empty fault means none of it may be written.
func Read(raw json.RawMessage, lineCount int) ([]Entry, string) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, "the reply carries no company profile"
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, "the company profile is not the required JSON shape"
	}
	seen := map[[2]string]bool{}
	for i := range entries {
		entry := &entries[i]
		entry.Value = strings.TrimSpace(entry.Value)
		switch {
		case !slices.Contains(Fields, entry.Field):
			return nil, fmt.Sprintf("the company profile names the field %.40q, which a meeting may not fill", entry.Field)
		case entry.SpeakerRef == "":
			return nil, fmt.Sprintf("the company profile's %s names no speaker", entry.Field)
		case seen[[2]string{entry.SpeakerRef, entry.Field}]:
			return nil, fmt.Sprintf("the company profile states %s twice for one speaker", entry.Field)
		case entry.Value == "":
			return nil, fmt.Sprintf("the company profile's %s is empty", entry.Field)
		case len([]rune(entry.Value)) > MaxValue:
			return nil, fmt.Sprintf("the company profile's %s is longer than %d characters", entry.Field, MaxValue)
		case entry.Confidence < 0 || entry.Confidence > 1:
			return nil, fmt.Sprintf("the company profile's %s has confidence %v, outside 0 to 1", entry.Field, entry.Confidence)
		}
		if msg := activities.TranscriptCitationFault(entry.SourceLines, lineCount, activities.MaxTranscriptCitedLines); msg != "" {
			return nil, fmt.Sprintf("the company profile's %s %s", entry.Field, msg)
		}
		seen[[2]string{entry.SpeakerRef, entry.Field}] = true
	}
	return entries, ""
}

// Apply writes what a transcript states about how the companies on the other
// side sell. Each value is said by a verified outside participant and lands on
// that contact's current employer. A refused reply leaves the earlier
// reading's values standing. A profile the reader may not update is skipped,
// so the reading still counts.
func Apply(
	ctx context.Context, tx pgx.Tx, log *slog.Logger, raw json.RawMessage, lineCount int,
	transcript ids.ActivityID, parties []commitmentparty.Party,
) error {
	entries, fault := Read(raw, lineCount)
	if fault != "" {
		log.WarnContext(ctx, "meeting company profile refused", "reason", fault)
		return nil
	}
	stated, err := statedByCompany(ctx, tx, entries, parties)
	if err == nil {
		err = contacts.ApplyMeetingProfileTx(ctx, tx, transcript, stated)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		log.InfoContext(ctx, "meeting company profile skipped", "reason", err)
		return nil
	}
	return err
}

// statedByCompany groups the entries by the employer of the outside contact
// who said each one. An entry whose speaker is a colleague, unverified, or
// employed nowhere today is left out. When two speakers of one company state a
// field, the higher confidence wins and a tie keeps the earlier entry.
func statedByCompany(
	ctx context.Context, tx pgx.Tx, entries []Entry, parties []commitmentparty.Party,
) (map[ids.CompanyID][]contacts.MeetingProfileField, error) {
	refs := commitmentparty.Refs(parties)
	said := map[int]ids.UUID{}
	var contactIDs []ids.UUID
	for i, entry := range entries {
		if contact, ok := commitmentparty.Outside(refs, entry.SpeakerRef, parties); ok {
			said[i] = contact
			contactIDs = append(contactIDs, contact)
		}
	}
	stated := map[ids.CompanyID][]contacts.MeetingProfileField{}
	if len(contactIDs) == 0 {
		return stated, nil
	}
	employers, err := contacts.CurrentEmployersTx(ctx, tx, contactIDs)
	if err != nil {
		return nil, err
	}
	slot := map[ids.CompanyID]map[string]int{}
	for i, entry := range entries {
		contact, spoke := said[i]
		employer, employed := employers[contact]
		if !spoke || !employed {
			continue
		}
		company := ids.From[ids.CompanyKind](ids.UUID(employer.CompanyId))
		field := contacts.MeetingProfileField{Field: entry.Field, Value: entry.Value, Confidence: entry.Confidence}
		if slot[company] == nil {
			slot[company] = map[string]int{}
		}
		if at, taken := slot[company][entry.Field]; taken {
			if entry.Confidence > stated[company][at].Confidence {
				stated[company][at] = field
			}
			continue
		}
		slot[company][entry.Field] = len(stated[company])
		stated[company] = append(stated[company], field)
	}
	return stated, nil
}
