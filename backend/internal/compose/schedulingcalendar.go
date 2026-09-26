// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type schedulingCalendar struct {
	registry *capture.Registry
	identity *identity.Service
	pool     *pgxpool.Pool
}

func newSchedulingCalendar(pool *pgxpool.Pool, registry *capture.Registry) schedulingCalendar {
	return schedulingCalendar{registry: registry, identity: identity.NewService(pool), pool: pool}
}

func (c schedulingCalendar) Check(ctx context.Context, host ids.UserID, provider string) error {
	if c.registry == nil {
		return apperrors.ErrPermissionDenied
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	rbac, seat, err := c.identity.EffectiveAuthority(ctx, workspace, host.UUID)
	if err != nil {
		return err
	}
	hostCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + host.String(), UserID: host.UUID,
		SeatType: seat, Permissions: rbac.Permissions, TeamIDs: rbac.TeamIDs,
	})
	if err := auth.Require(hostCtx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	_, _, err = c.registry.CalendarFor(ctx, host, provider, true)
	if errors.Is(err, capture.ErrNoConnection) {
		return connector.ErrAuthRejected
	}
	return err
}

func (c schedulingCalendar) Busy(ctx context.Context, host ids.UserID, provider, calendar string, from, to time.Time) ([]connector.CalendarInterval, error) {
	if c.registry == nil {
		return nil, apperrors.ErrPermissionDenied
	}
	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, false)
	if err != nil {
		return nil, err
	}
	return scheduler.CalendarBusy(ctx, credential, calendar, from, to)
}

func (c schedulingCalendar) Save(ctx context.Context, host ids.UserID, provider string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	if err := c.checkCalendar(ctx, host, provider, in.CalendarID); err != nil {
		return connector.CalendarReceipt{}, err
	}
	if !in.PassportID.IsZero() {
		live, err := c.identity.AuthenticateAgentByID(ctx, ids.From[ids.PassportKind](in.PassportID))
		if err != nil {
			return connector.CalendarReceipt{}, err
		}
		if live.OnBehalfOf != host || !live.Scopes.Has(principal.ScopeSend) {
			return connector.CalendarReceipt{}, apperrors.ErrPermissionDenied
		}
	}
	for _, address := range in.Attendees {
		if err := c.CheckRecipient(ctx, host, in.ContactID, address); err != nil {
			return connector.CalendarReceipt{}, err
		}
	}
	if err := consentGateFor(c.pool).RequireGrantedForEmails(ctx, in.Attendees, "transactional"); err != nil {
		return connector.CalendarReceipt{}, err
	}

	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, true)
	if err != nil {
		return connector.CalendarReceipt{}, err
	}
	return scheduler.CalendarSave(ctx, credential, in)
}

func (c schedulingCalendar) Cancel(ctx context.Context, host ids.UserID, provider, calendar, event string) error {
	if err := c.checkCalendar(ctx, host, provider, calendar); err != nil {
		return err
	}
	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, true)
	if err != nil {
		return err
	}
	return scheduler.CalendarCancel(ctx, credential, calendar, event)
}

func (c schedulingCalendar) Lookup(ctx context.Context, host ids.UserID, provider string, in connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	if err := c.checkCalendar(ctx, host, provider, in.CalendarID); err != nil {
		return nil, err
	}
	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, false)
	if err != nil {
		return nil, err
	}
	return scheduler.CalendarLookup(ctx, credential, in)
}

func (c schedulingCalendar) CheckRecipient(ctx context.Context, host ids.UserID, contactID ids.UUID, address string) error {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	rbac, seat, err := c.identity.EffectiveAuthority(ctx, workspace, host.UUID)
	if err != nil {
		return err
	}
	hostCtx := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + host.String(), UserID: host.UUID, SeatType: seat, Permissions: rbac.Permissions, TeamIDs: rbac.TeamIDs})
	contact, err := contacts.NewStore(InstallationDB(c.pool)).GetContact(hostCtx, ids.From[ids.ContactKind](contactID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	if contact.Emails != nil {
		for _, email := range *contact.Emails {
			if strings.EqualFold(string(email.Email), address) {
				return nil
			}
		}
	}
	return &activities.SchedulingArgumentError{Field: "attendee_email", Code: "recipient_mismatch", Message: "Add the attendee's email to this contact before sending an invitation"}
}

func (c schedulingCalendar) List(ctx context.Context, host ids.UserID, provider string) ([]connector.CalendarOption, error) {
	if c.registry == nil {
		return nil, apperrors.ErrPermissionDenied
	}
	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, false)
	if err != nil {
		return nil, err
	}
	return scheduler.CalendarList(ctx, credential)
}

func (c schedulingCalendar) Inspect(ctx context.Context, host ids.UserID, provider, calendar, event string) (connector.CalendarState, error) {
	if err := c.checkCalendar(ctx, host, provider, calendar); err != nil {
		return connector.CalendarState{}, err
	}
	scheduler, credential, err := c.registry.CalendarFor(ctx, host, provider, false)
	if err != nil {
		return connector.CalendarState{}, err
	}
	return scheduler.CalendarInspect(ctx, credential, calendar, event)
}

func (c schedulingCalendar) checkCalendar(ctx context.Context, host ids.UserID, provider, id string) error {
	if err := c.Check(ctx, host, provider); err != nil {
		return err
	}
	// An account-relative alias cannot prove that this is the original calendar.
	if id == "primary" {
		return connector.ErrAuthRejected
	}
	calendars, err := c.List(ctx, host, provider)
	if err != nil {
		return err
	}
	for _, calendar := range calendars {
		if calendar.ID == id && calendar.Writable {
			return nil
		}
	}
	return connector.ErrAuthRejected
}
