// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// attendeeCandidates are the records that hold one address. contacts are
// already bounded by the host's row scope. held is the consent gate's answer
// to whether any contact or lead holds it, seen or not.
type attendeeCandidates struct {
	contacts      []ids.UUID
	colleague     ids.UUID
	colleagueName string
	leads         []ids.UUID
	held          bool
}

// ResolveAttendees resolves each address contact first, then live colleague,
// then lead, under the host's row scope. Contact first keeps a contact who
// withdrew consent from passing as a colleague. An address any contact or lead
// holds asks the consent gate even when the host cannot see that record.
func (c schedulingCalendar) ResolveAttendees(ctx context.Context, host ids.UserID, addresses []string) ([]connector.CalendarAttendee, error) {
	hostCtx, err := c.recipientContext(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]connector.CalendarAttendee, 0, len(addresses))
	for _, typed := range addresses {
		address := connector.AttendeeAddress(typed)
		found, err := c.attendeeCandidates(ctx, hostCtx, address)
		if err != nil {
			return nil, err
		}
		found.held, err = consentGateFor(c.pool).AddressHasSubject(ctx, address)
		if err != nil {
			return nil, err
		}
		attendee, err := c.resolveAttendee(hostCtx, address, found)
		if err != nil {
			return nil, err
		}
		out = append(out, attendee)
	}
	return out, nil
}

func (c schedulingCalendar) resolveAttendee(hostCtx context.Context, address string, found attendeeCandidates) (connector.CalendarAttendee, error) {
	held := found.held
	store := contacts.NewStore(InstallationDB(c.pool))
	for _, id := range found.contacts {
		contact, err := store.GetContact(hostCtx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
		if visible, err := hostSees(err); err != nil || visible {
			return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeContact, ID: id, Name: contact.FullName, AskConsent: true}, err
		}
	}
	if !found.colleague.IsZero() {
		return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeColleague, ID: found.colleague, Name: found.colleagueName, AskConsent: held}, nil
	}
	for _, id := range found.leads {
		_, err := store.GetLead(hostCtx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if visible, err := hostSees(err); err != nil || visible {
			// No name: a lead's participant row is an address row, which no
			// lead scope guards for the meeting's other readers.
			return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeLead, ID: id, AskConsent: true}, err
		}
	}
	return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeHostNamed, AskConsent: held}, nil
}

// hostSees reads a row-scoped get as visible or not; a miss and a denial both
// mean the host may not name that record.
func hostSees(err error) (bool, error) {
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

func (c schedulingCalendar) attendeeCandidates(ctx, hostCtx context.Context, address string) (attendeeCandidates, error) {
	args := []any{address, owndomain.OfAddress(address)}
	scope, err := auth.ScopeClauseFor(hostCtx, "contact", "c", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return attendeeCandidates{}, err
	}
	switch {
	case !auth.ReadGranted(hostCtx, "contact"):
		scope = "false"
	case scope == "":
		scope = "true"
	}
	var found attendeeCandidates
	err = database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT 'contact', e.contact_id, '' FROM contact_email e JOIN contact c ON c.id = e.contact_id
			 WHERE e.email = $1 AND e.archived_at IS NULL AND c.archived_at IS NULL AND (`+scope+`)
			UNION SELECT 'user', u.id, u.display_name FROM app_user u WHERE `+storekit.SeatLoginIsSQL("u", "$1", "$2")+` AND `+workforce.LiveHumanSQL("u")+`
			UNION SELECT 'lead', l.id, '' FROM lead l WHERE l.email = $1 AND l.archived_at IS NULL AND l.promoted_contact_id IS NULL
			ORDER BY 1, 2`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kind, name string
			var id ids.UUID
			if err := rows.Scan(&kind, &id, &name); err != nil {
				return err
			}
			switch kind {
			case "contact":
				found.contacts = append(found.contacts, id)
			case "user":
				found.colleague, found.colleagueName = id, name
			default:
				found.leads = append(found.leads, id)
			}
		}
		return rows.Err()
	})
	return found, err
}

// consentAddresses checks every attendee once more before the provider sends,
// and answers the addresses the consent gate must clear. Any attendee whose
// address carries a stop that binds an invitation refuses the whole send. The primary invitee
// is checked against its own contact or lead. A further attendee is resolved
// afresh, and the stricter of its queued and its current answer wins. An
// address gated when it was queued stays gated after its contact or lead lost
// it. The gate then refuses it as a subject it cannot find.
func (c schedulingCalendar) consentAddresses(ctx context.Context, host ids.UserID, in connector.CalendarAppointment) ([]string, error) {
	gate := consentGateFor(c.pool)
	gated := make([]string, 0, len(in.Attendees))
	for i, address := range in.Attendees {
		// A stop on the address binds every attendee, whatever record holds it.
		suppressed, err := gate.InvitationSuppressed(ctx, address)
		if err != nil {
			return nil, err
		}
		if suppressed {
			return nil, fmt.Errorf("compose: %s is suppressed: %w", address, apperrors.ErrConsentNotGranted)
		}
		if i == 0 || len(in.Roster) != len(in.Attendees) {
			if err := c.checkAppointmentRecipient(ctx, host, in, address); err != nil {
				return nil, err
			}
			gated = append(gated, address)
			continue
		}
		resolved, err := c.ResolveAttendees(ctx, host, []string{address})
		if err != nil {
			return nil, err
		}
		queued := in.Roster[i]
		if queued.AskConsent || !strings.EqualFold(queued.Address, address) || resolved[0].AskConsent {
			gated = append(gated, address)
		}
	}
	return gated, nil
}

// InviteeAddress is the address a booking's invitee is invited at, read under
// the host's row scope. It is the first typed address their record holds,
// else the address the contact is reachable at. A promoted lead answers its
// contact, and the second result names the contact the invitation files on.
func (c schedulingCalendar) InviteeAddress(ctx context.Context, host ids.UserID, entityType string, id ids.UUID, typed []string) (string, ids.UUID, error) {
	hostCtx, err := c.recipientContext(ctx, host)
	if err != nil {
		return "", ids.Nil, err
	}
	store := contacts.NewStore(InstallationDB(c.pool))
	if entityType == string(crmcontracts.ActivityLinkEntityTypeLead) {
		lead, err := store.GetLead(hostCtx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			return "", ids.Nil, err
		}
		if lead.PromotedContactId == nil {
			if lead.Email == nil {
				return "", ids.Nil, nil
			}
			return string(*lead.Email), ids.Nil, nil
		}
		id = ids.UUID(*lead.PromotedContactId)
	}
	contact, err := store.GetContact(hostCtx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
	if err != nil || contact.Emails == nil {
		return "", id, err
	}
	for _, address := range typed {
		for _, email := range *contact.Emails {
			if strings.EqualFold(string(email.Email), address) {
				return address, id, nil
			}
		}
	}
	if contact.PrimaryEmail == nil {
		return "", id, nil
	}
	return string(*contact.PrimaryEmail), id, nil
}

// Blocker says why the host's calendar cannot take bookings. It reads the
// connection's granted scopes and the host's authority, and calls no provider.
func (c schedulingCalendar) Blocker(ctx context.Context, host ids.UserID, provider string) (crmcontracts.SchedulingProfileBlocker, error) {
	if c.registry == nil {
		return crmcontracts.SchedulingProfileBlockerCalendarUnavailable, nil
	}
	scopes, err := c.registry.GrantedScopesFor(ctx, host, provider)
	if errors.Is(err, capture.ErrNoConnection) {
		return crmcontracts.SchedulingProfileBlockerCalendarUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if !capture.CalendarWriteGranted(provider, scopes) {
		return crmcontracts.SchedulingProfileBlockerCalendarReadOnly, nil
	}
	err = c.Check(ctx, host, provider)
	if errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return crmcontracts.SchedulingProfileBlockerCalendarUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	return crmcontracts.SchedulingProfileBlockerNone, nil
}

// ReconnectedSince reads the connection's grant time; no provider call.
func (c schedulingCalendar) ReconnectedSince(ctx context.Context, host ids.UserID, provider string, since time.Time) (bool, error) {
	if c.registry == nil {
		return false, nil
	}
	return c.registry.ReconnectedSince(ctx, host, provider, since)
}
