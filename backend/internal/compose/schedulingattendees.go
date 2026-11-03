// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
// already bounded by the host's row scope; held says whether any live contact
// holds it at all, seen or not.
type attendeeCandidates struct {
	contacts  []ids.UUID
	colleague ids.UUID
	leads     []ids.UUID
	held      bool
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
	for _, address := range addresses {
		found, err := c.attendeeCandidates(ctx, hostCtx, address)
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
	held := found.held || len(found.leads) > 0
	store := contacts.NewStore(InstallationDB(c.pool))
	for _, id := range found.contacts {
		visible, err := hostSees(store.GetContact(hostCtx, ids.From[ids.ContactKind](id), storekit.LiveOnly))
		if err != nil || visible {
			return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeContact, ID: id, AskConsent: true}, err
		}
	}
	if !found.colleague.IsZero() {
		return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeColleague, ID: found.colleague, AskConsent: held}, nil
	}
	for _, id := range found.leads {
		visible, err := hostSees(store.GetLead(hostCtx, ids.From[ids.LeadKind](id), storekit.LiveOnly))
		if err != nil || visible {
			return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeLead, ID: id, AskConsent: true}, err
		}
	}
	return connector.CalendarAttendee{Address: address, Basis: connector.AttendeeHostNamed, AskConsent: held}, nil
}

// hostSees reads a row-scoped get as visible or not; a miss and a denial both
// mean the host may not name that record.
func hostSees[T any](_ T, err error) (bool, error) {
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
			SELECT 'contact', e.contact_id FROM contact_email e JOIN contact c ON c.id = e.contact_id
			 WHERE e.email = $1 AND e.archived_at IS NULL AND c.archived_at IS NULL AND (`+scope+`)
			UNION SELECT 'held', '00000000-0000-0000-0000-000000000000'::uuid
			 WHERE EXISTS (SELECT 1 FROM contact_email WHERE email = $1 AND archived_at IS NULL)
			UNION SELECT 'user', u.id FROM app_user u WHERE `+storekit.SeatLoginIsSQL("u", "$1", "$2")+` AND `+workforce.LiveHumanSQL("u")+`
			UNION SELECT 'lead', l.id FROM lead l WHERE l.email = $1 AND l.archived_at IS NULL AND l.promoted_contact_id IS NULL
			ORDER BY 1, 2`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kind string
			var id ids.UUID
			if err := rows.Scan(&kind, &id); err != nil {
				return err
			}
			switch kind {
			case "contact":
				found.contacts = append(found.contacts, id)
			case "held":
				found.held = true
			case "user":
				found.colleague = id
			default:
				found.leads = append(found.leads, id)
			}
		}
		return rows.Err()
	})
	return found, err
}

// consentAddresses checks every attendee once more before the provider sends,
// and answers the addresses the consent gate must clear. The primary invitee
// is checked against its own contact or lead. A further attendee is resolved
// afresh, and the stricter of its queued and its current answer wins. An
// address gated when it was queued stays gated after its contact or lead lost
// it. The gate then refuses it as a subject it cannot find.
func (c schedulingCalendar) consentAddresses(ctx context.Context, host ids.UserID, in connector.CalendarAppointment) ([]string, error) {
	gated := make([]string, 0, len(in.Attendees))
	for i, address := range in.Attendees {
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
