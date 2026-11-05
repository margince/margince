// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Somebody is a lead or a contact, never both. A contact write is refused when
// it gives a contact an address or LinkedIn profile that a live lead holds. The
// one exception is a write that converts that lead in the same transaction. The
// question is storekit's (LockIdentityHolders); the answer for a contact is here.

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// CodeLeadHoldsEmail is the wire code of every lead-held refusal.
const CodeLeadHoldsEmail = "lead_holds_email"

// The request fields a lead-held refusal names.
const (
	fieldEmails         = "emails"
	fieldSocialLinkedIn = "social.linkedin"
	fieldConvertLeadIDs = "convert_lead_ids"
)

// LeadHoldsIdentityError refuses a contact write over a key a live lead holds.
// LeadIDs names only the leads the caller may read. Convertible says the caller
// may resend with convert_lead_ids: every holder is named and they hold both
// grants a promotion needs.
type LeadHoldsIdentityError struct {
	Field       string
	LeadIDs     []ids.LeadID
	Convertible bool
}

func (e *LeadHoldsIdentityError) Error() string {
	if e.Field == fieldSocialLinkedIn || e.Field == profileURLRequestField {
		return "a live lead already holds this LinkedIn profile; convert the lead instead"
	}
	return "a live lead already holds this email address; convert the lead instead"
}

// Is maps the refusal onto the shared conflict sentinel, as the other
// duplicate refusals do.
func (e *LeadHoldsIdentityError) Is(target error) bool { return target == apperrors.ErrConflict }

// MessageFault names the code for a door that maps no contacts error itself,
// such as a review accept answered through approvals.
func (e *LeadHoldsIdentityError) MessageFault() (code, message string) {
	return CodeLeadHoldsEmail, e.Error()
}

// contactKeys are the identity keys a contact write would give a contact.
func contactKeys(emails []ContactEmailInput, social map[string]any) storekit.IdentityKeys {
	var keys storekit.IdentityKeys
	for _, e := range emails {
		keys.Emails = append(keys.Emails, e.Email)
	}
	if url, ok := linkedInIdentity(social); ok {
		keys.LinkedInURLs = []string{url}
	}
	return keys
}

// linkedInIdentity is the profile URL a social map names, in the spelling
// contactSocialInputs stores. A handle that is not a readable profile URL is
// stored as typed and names nobody, so it is no key.
func linkedInIdentity(social map[string]any) (string, bool) {
	rows, err := contactSocialInputs(social)
	if err != nil {
		return "", false
	}
	for _, row := range rows {
		if row.Platform == SocialLinkedIn {
			return row.Handle, isLinkedinURL(row.Handle)
		}
	}
	return "", false
}

// refuseLeadHolders answers the leads among holders that this write does not
// convert, or nil when there are none.
func refuseLeadHolders(ctx context.Context, tx pgx.Tx, holders storekit.IdentityHolders, keys storekit.IdentityKeys, converting []ids.LeadID) error {
	var refused []ids.LeadID
	for _, lead := range holders.Leads {
		if !slices.Contains(converting, lead) {
			refused = append(refused, lead)
		}
	}
	if len(refused) == 0 {
		return nil
	}
	field := fieldSocialLinkedIn
	for _, email := range keys.Emails {
		if holders.LeadHoldsEmail(email) {
			field = fieldEmails
		}
	}
	return leadHoldsRefusal(ctx, tx, field, refused)
}

// leadHoldsRefusal builds the refusal, naming each lead only when the caller
// may read it (nameableLead). A hidden holder leaves the convert unoffered,
// because the caller cannot name the lead it would convert.
func leadHoldsRefusal(ctx context.Context, tx pgx.Tx, field string, leads []ids.LeadID) error {
	refusal := &LeadHoldsIdentityError{Field: field, LeadIDs: []ids.LeadID{}}
	for _, lead := range leads {
		named, err := nameableLead(ctx, tx, lead)
		if err != nil {
			return err
		}
		if !named.IsZero() {
			refusal.LeadIDs = append(refusal.LeadIDs, named)
		}
	}
	// A hidden holder is told no more than a hidden contact holder's refusal
	// tells. The key is taken; not which key, and not by whom.
	if len(refusal.LeadIDs) != len(leads) {
		refusal.LeadIDs = []ids.LeadID{}
		refusal.Field = ""
		return refusal
	}
	allowed, err := mayPromoteHeldLeads(ctx)
	if err != nil {
		return err
	}
	refusal.Convertible = allowed && len(leads) == 1
	return refusal
}

// admitLeadHolders asks, under the identity locks, who holds the keys a typed
// contact write gives. It answers the leads this write converts. A live lead
// the caller did not name in convert is the refusal. The named set must equal
// the holders as they stand now. A holder that changed since is a new question.
func admitLeadHolders(ctx context.Context, tx pgx.Tx, keys storekit.IdentityKeys, convert []ids.LeadID) ([]ids.LeadID, error) {
	holders, err := storekit.LockIdentityHolders(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	if len(holders.Leads) == 0 {
		if len(convert) > 0 {
			return nil, &LeadHoldsIdentityError{LeadIDs: []ids.LeadID{}}
		}
		return nil, nil
	}
	if len(holders.Leads) != 1 {
		return nil, refuseLeadHolders(ctx, tx, holders, keys, nil)
	}
	if !sameLeads(holders.Leads, convert) {
		return nil, refuseLeadHolders(ctx, tx, holders, keys, nil)
	}
	allowed, err := mayPromoteHeldLeads(ctx)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, apperrors.ErrPermissionDenied
	}
	return holders.Leads, nil
}

func sameLeads(holders, convert []ids.LeadID) bool {
	if len(convert) == 0 {
		return false
	}
	named := slices.Clone(convert)
	slices.SortFunc(named, compareLeads)
	named = slices.Compact(named)
	held := slices.Clone(holders)
	slices.SortFunc(held, compareLeads)
	return slices.Equal(held, named)
}

func compareLeads(a, b ids.LeadID) int { return bytes.Compare(a.UUID[:], b.UUID[:]) }

// refuseAgentConvert keeps a conversion a human's act. An agent that meets the
// refusal calls promote_lead; it never converts as the side effect of a write.
func refuseAgentConvert(ctx context.Context, convert []ids.LeadID) error {
	if len(convert) == 0 {
		return nil
	}
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalAgent {
		return &values.ParseError{
			Field: fieldConvertLeadIDs, Code: "convert_needs_a_human",
			Message: "an agent converts a lead with promote_lead, not with a contact write",
		}
	}
	return nil
}

// convertLeadsIntoTx converts each lead into the contact this write landed on,
// through the ordinary promotion. A refusal fails the whole write: the user
// asked for the conversion, so a contact without it is not what they asked for.
func (s *Store) convertLeadsIntoTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, leads []ids.LeadID, by string) error {
	for _, lead := range leads {
		out, err := s.promoteLeadTx(ctx, tx, lead, PromoteLeadInput{
			Trigger: string(TriggerHumanQualify), intoCandidate: true,
		}, by, nil, contactID)
		if err != nil {
			return err
		}
		if !out.Merged || ids.UUID(out.Contact.Id) != contactID.UUID {
			return apperrors.ErrConflict
		}
	}
	return nil
}

// refuseLeadHeldLinkedIn refuses a LinkedIn profile a live lead holds, when the
// write gives it to this contact anew. A profile the contact already holds is
// not asked about again, so a save of an untouched form still lands.
func refuseLeadHeldLinkedIn(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, social map[string]any, converting []ids.LeadID) error {
	url, ok := linkedInIdentity(social)
	if !ok {
		return nil
	}
	var holds bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact_social
		 WHERE contact_id = $1 AND platform = $2 AND handle = $3)`, contactID, SocialLinkedIn, url).Scan(&holds); err != nil {
		return fmt.Errorf("read the contact's LinkedIn profile: %w", err)
	}
	if holds {
		return nil
	}
	keys := storekit.IdentityKeys{LinkedInURLs: []string{url}}
	holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
	if err != nil {
		return err
	}
	return refuseLeadHolders(ctx, tx, holders, keys, converting)
}

// admitEditLeadHolders asks who holds the keys an edit gives the contact anew,
// and records the leads the edit converts. Keys the contact already holds are
// not asked about, so editing a contact that already shares an address with a
// lead still saves. The contact row is locked before the identity locks.
func admitEditLeadHolders(ctx context.Context, tx pgx.Tx, id ids.ContactID, current crmcontracts.Contact, in *UpdateContactInput) error {
	if err := refuseAgentConvert(ctx, in.ConvertLeadIDs); err != nil {
		return err
	}
	keys := freshEditKeys(current, *in)
	if len(keys.Emails) == 0 && len(keys.LinkedInURLs) == 0 {
		if len(in.ConvertLeadIDs) > 0 {
			return &LeadHoldsIdentityError{LeadIDs: []ids.LeadID{}}
		}
		return nil
	}
	if _, err := storekit.LockRow(ctx, tx, contactEntity, id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	converting, err := admitLeadHolders(ctx, tx, keys, in.ConvertLeadIDs)
	in.converting = converting
	return err
}

// freshEditKeys are the addresses and the profile an edit gives the contact
// that it does not hold yet.
func freshEditKeys(current crmcontracts.Contact, in UpdateContactInput) storekit.IdentityKeys {
	held := map[string]bool{}
	if current.Emails != nil {
		for _, e := range *current.Emails {
			held[strings.ToLower(string(e.Email))] = true
		}
	}
	var keys storekit.IdentityKeys
	for _, e := range in.Emails {
		if !held[strings.ToLower(strings.TrimSpace(e.Email))] {
			keys.Emails = append(keys.Emails, e.Email)
		}
	}
	if in.Social == nil {
		return keys
	}
	url, ok := linkedInIdentity(in.Social)
	if !ok {
		return keys
	}
	if current.Social != nil {
		if stored, ok := linkedInIdentity(*current.Social); ok && stored == url {
			return keys
		}
	}
	keys.LinkedInURLs = []string{url}
	return keys
}

// lockConvertStops takes consent's stop locks of a contact and the leads an
// edit converts into it, before the contact row. The promotion takes them
// again. Taking them after the row inverts the order a stop write takes.
func (s *Store) lockConvertStops(ctx context.Context, tx pgx.Tx, id ids.ContactID, convert []ids.LeadID) error {
	if len(convert) == 0 {
		return nil
	}
	subjects := []commsauthz.StopSubject{commsauthz.ContactStopSubject(id)}
	for _, lead := range convert {
		subjects = append(subjects, commsauthz.LeadStopSubject(lead))
	}
	return lockStopsOrSkip(ctx, tx, s.stopCarrier, subjects...)
}

// convertEditedLeads converts the leads an edit was admitted to convert, once
// the contact holds their keys.
func (s *Store) convertEditedLeads(ctx context.Context, tx pgx.Tx, id ids.ContactID, converting []ids.LeadID) error {
	if len(converting) == 0 {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.convertLeadsIntoTx(ctx, tx, id, converting, by)
}

// leadHolds answers whether a live lead holds any of these keys, for a machine
// write that leaves the lead as it is instead of asking.
func leadHolds(ctx context.Context, tx pgx.Tx, keys storekit.IdentityKeys) (bool, error) {
	holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
	return len(holders.Leads) > 0, err
}

// refuseLeadHeldEmails is the contact address writer's backstop: a live lead's
// address is refused unless this transaction converts that lead.
func refuseLeadHeldEmails(ctx context.Context, tx pgx.Tx, addresses []string, converting []ids.LeadID) error {
	keys := storekit.IdentityKeys{Emails: addresses}
	holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
	if err != nil {
		return err
	}
	return refuseLeadHolders(ctx, tx, holders, keys, converting)
}
