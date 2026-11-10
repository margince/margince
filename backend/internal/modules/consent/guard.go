// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The contact summary reports facts per purpose. It shares the underlying
// evidence readers with sending; each message still needs its own send check.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetContactConsentGuard implements GET /contacts/{id}/consent/guard.
func (h Handlers) GetContactConsentGuard(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	guard, err := h.store.ContactConsentGuard(r.Context(), pathID[ids.ContactKind](id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, guard)
}

// ContactConsentGuard reports current evidence for each configured purpose.
func (s *Store) ContactConsentGuard(ctx context.Context, contactID ids.ContactID) (crmcontracts.ContactConsentGuard, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.ContactConsentGuard{}, err
	}
	out := crmcontracts.ContactConsentGuard{
		ContactId: openapi_types.UUID(contactID.UUID),
		Entries:   []crmcontracts.ContactConsentGuardEntry{},
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Anything that names a record is gated: answering about a contact the
		// caller cannot read would confirm that contact exists.
		if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		purposes, err := PurposesForGuard(ctx, tx)
		if err != nil {
			return err
		}
		// The SAME window the send path binds a decision to. A preview that
		// answered on a different span would tell a rep a send is allowed that
		// the engine then refuses, which is worse than no preview at all.
		country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: entityContact, ID: contactID.String()})
		if err != nil {
			return err
		}
		w := packRulesFor(country)
		asOf := s.now()
		since := asOf.Add(-w.reply)
		// The same marketing context the send path builds for this caller, so
		// the advisory answer and the send agree on the existing-customer route.
		marketing, err := marketingContextTx(ctx, tx, w)
		if err != nil {
			return err
		}
		for _, purpose := range purposes {
			verdict, err := GuardVerdictForContact(ctx, tx, contactID.String(), purpose, since, asOf, marketing)
			if err != nil {
				return err
			}
			entry, err := wireGuardEntry(purpose, verdict)
			if err != nil {
				return err
			}
			out.Entries = append(out.Entries, entry)
		}
		return nil
	})
	if err != nil {
		return crmcontracts.ContactConsentGuard{}, err
	}
	return out, nil
}

func wireGuardEntry(purpose PurposeRow, verdict Verdict) (crmcontracts.ContactConsentGuardEntry, error) {
	label := purpose.Label
	entry := crmcontracts.ContactConsentGuardEntry{
		PurposeKey:   purpose.Key,
		PurposeLabel: &label,
		PurposeClass: crmcontracts.ContactConsentGuardEntryPurposeClass(purpose.Class),
		Channel:      channelFor(purpose.Class),
		Verdict:      crmcontracts.ContactConsentGuardEntryVerdict(verdict.State),
		Reason:       verdict.Reason,
		ReasonCode:   crmcontracts.ContactConsentGuardEntryReasonCode(verdict.ReasonCode),
		ReasonAt:     verdict.ReasonAt,
	}
	if verdict.Qualifying != nil {
		qualifying, err := wireQualifying(*verdict.Qualifying)
		if err != nil {
			return crmcontracts.ContactConsentGuardEntry{}, err
		}
		entry.QualifyingEvent = qualifying
	}
	return entry, nil
}

// channelFor names the channel a purpose is actually sent on. Only the phone
// class is not mail, and nothing sends on it yet.
func channelFor(class Class) crmcontracts.ContactConsentGuardEntryChannel {
	if class == ClassPhoneOutreach {
		return crmcontracts.ContactConsentGuardEntryChannelPhone
	}
	return crmcontracts.ContactConsentGuardEntryChannelEmail
}

func wireQualifying(event QualifyingEvent) (*crmcontracts.ConsentQualifyingEvent, error) {
	wired := crmcontracts.ConsentQualifyingEvent{
		Kind:       crmcontracts.ConsentQualifyingEventKind(event.Kind),
		OccurredAt: event.OccurredAt,
	}
	if event.ID != "" {
		id, err := ids.Parse(event.ID)
		if err != nil {
			return nil, fmt.Errorf("the qualifying event has an unreadable id: %w", err)
		}
		wired.Id = new(openapi_types.UUID(id))
	}
	if event.SourceEntityType != "" {
		sourceType := crmcontracts.ConsentQualifyingEventSourceEntityType(event.SourceEntityType)
		wired.SourceEntityType = &sourceType
	}
	if event.SourceEntityID != "" {
		parsed, err := ids.Parse(event.SourceEntityID)
		if err != nil {
			// The column is a uuid and the DDL requires it beside its type, so
			// an unparseable value means the row is not what the schema says it
			// is. Rendering the verdict without its proof would present an
			// unevidenced "allowed" as an evidenced one.
			return nil, fmt.Errorf("the qualifying event names an unreadable source record: %w", err)
		}
		sourceID := openapi_types.UUID(parsed)
		wired.SourceEntityId = &sourceID
	}
	if event.Note != "" {
		note := event.Note
		wired.Note = &note
	}
	return &wired, nil
}
