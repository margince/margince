// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MeetingDeliveryArgs carries no tenant data; each pass resolves the installation.
type MeetingDeliveryArgs struct{}

// Kind names the declared periodic delivery job.
func (MeetingDeliveryArgs) Kind() string { return "meeting_delivery" }

// InsertOpts uses the job catalog’s retry and queue policy.
func (MeetingDeliveryArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: river.QueueDefault, MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByState: activeSweepStates}}
}

type meetingDeliveryWorker struct {
	pool      *pgxpool.Pool
	registry  *capture.Registry
	vault     keyvault.Vault
	origin    SendOrigin
	reminders *scheduledSendWorker
}

func (w *meetingDeliveryWorker) Work(ctx context.Context, _ *river.Job[MeetingDeliveryArgs]) error {
	pass, err := installationJobCtx(ctx, identity.NewService(w.pool))
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	pass = principal.SystemActing(pass, "meeting_delivery")
	store := activities.NewStore(InstallationDB(w.pool)).WithClock(time.Now).WithMeetingVault(w.vault).WithPublicBaseURL(w.origin.PublicBaseURL).WithRuntimeEnvironment(w.origin.Environment).
		WithWorkingHours(workingHoursResolver(w.pool)).WithSchedulingCalendar(newSchedulingCalendar(w.pool, w.registry))
	deliveryErr := store.DeliverInvitations(pass)
	reconcileErr := store.ReconcileInvitations(pass)
	return jobs.FaultContext(pass, errors.Join(deliveryErr, reconcileErr, w.remind(pass, store)))
}

func addMeetingDeliveryJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	if cfg.GmailRegistry == nil {
		return nil
	}
	reminders := newScheduledSendWorker(pool, cfg.SendDelivery, cfg.SendBlob, cfg.SendPacing, cfg.SendOrigin)
	reminders.store = reminders.store.WithSendAuthority(workerMailAuthority(cfg.SendRegistry))
	addDeclaredWorker[MeetingDeliveryArgs](reg, &meetingDeliveryWorker{pool: pool, registry: cfg.GmailRegistry, vault: cfg.ControllerVault, origin: cfg.SendOrigin, reminders: reminders})
	return periodicFor(cfg, MeetingDeliveryArgs{})
}

func (w *meetingDeliveryWorker) remind(ctx context.Context, store *activities.Store) error {
	due, err := store.DueMeetingReminders(ctx)
	if err != nil {
		return err
	}
	var faults []error
	for _, reminder := range due {
		fireCtx, refused, err := w.reminders.fireAs(ctx, schedulerOf{UserID: reminder.Host.UUID, Kind: "human"})
		if err != nil {
			faults = append(faults, err)
			continue
		}
		if refused != "" {
			if err := store.RefuseMeetingReminder(ctx, reminder); err != nil {
				faults = append(faults, err)
			}
			continue
		}
		sender := w.reminders.store.WithClock(time.Now).WithWorkingHours(workingHoursResolver(w.pool)).WithMeetingVault(w.vault).WithSchedulingCalendar(newSchedulingCalendar(w.pool, w.registry))
		if err := sender.SendMeetingReminder(fireCtx, reminder.ID, w.reminders.consent, w.reminders.delivery); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				err = errors.Join(err, store.RefuseMeetingReminder(ctx, reminder))
			}
			faults = append(faults, err)
		}
	}
	return errors.Join(faults...)
}
