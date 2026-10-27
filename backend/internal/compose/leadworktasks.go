// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two Worklist tasks that keep one human on one record. A lead who writes
// to us or books a meeting stays a lead, and its owner is asked to qualify
// them. A lead that arrives for somebody who is already a contact is not
// created, and the contact's owner is told what arrived and where from.
//
// Both are tasks, which are activities, and the contacts module may not write
// them, so the edge sits here.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// leadRepliedActor is the system principal the qualify task is written as.
const leadRepliedActor = "system:lead-replied"

// The task titles. A lead's name fills the first verb.
const (
	leadRepliedSubject    = "Review qualification for %s"
	leadBookedSubject     = "Review qualification for %s after booking"
	leadForContactSubject = "%s came in as a new lead, and is already your contact"
)

// leadWorkTasks writes both tasks. One writer, so a reply and a booking ask the
// lead's owner the same question.
type leadWorkTasks struct {
	db  *database.DB
	now func() time.Time
	log *slog.Logger
}

func newLeadWorkTasks(db *database.DB, log *slog.Logger) leadWorkTasks {
	return leadWorkTasks{db: db, now: time.Now, log: log}
}

// FileLeadReplyTx files the qualify task in the transaction that files the
// inbound message on the lead.
func (w leadWorkTasks) FileLeadReplyTx(ctx context.Context, tx pgx.Tx, lead ids.LeadID, arrival ids.UUID) error {
	return w.askToQualifyTx(principal.SystemActing(ctx, leadRepliedActor), tx, lead, arrival, leadRepliedSubject)
}

// FileLeadBookingTx files the qualify task in the booking transaction.
func (w leadWorkTasks) FileLeadBookingTx(ctx context.Context, tx pgx.Tx, lead ids.LeadID, arrival ids.UUID) error {
	return w.askToQualifyTx(principal.SystemActing(ctx, leadRepliedActor), tx, lead, arrival, leadBookedSubject)
}

func (w leadWorkTasks) askToQualifyTx(ctx context.Context, tx pgx.Tx, lead ids.LeadID, arrival ids.UUID, subject string) error {
	var name string
	var email *string
	var owner *ids.UUID
	err := tx.QueryRow(ctx, `SELECT COALESCE(full_name, ''), email, owner_id FROM lead WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, lead).Scan(&name, &email, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("compose: reading the lead for its qualify task: %w", err)
	}
	if err := storekit.LockWriteIdentity(ctx, tx, provenance.LeadRepliedSource, lead.String()); err != nil {
		return err
	}
	eligible, err := activities.CurrentLeadEngagementTx(ctx, tx, arrival, owner, w.now())
	if err != nil || !eligible {
		return err
	}
	open, err := activities.OpenSystemTaskOnTx(ctx, tx, provenance.LeadRepliedSource, entityLead, lead.UUID, w.now(), owner)
	if err != nil || open {
		return err
	}
	if strings.TrimSpace(name) == "" && email != nil {
		name = *email
	}
	if strings.TrimSpace(name) == "" {
		name = "A lead"
	}
	title := fmt.Sprintf(subject, strings.TrimSpace(name))
	return w.logSystemTaskTx(ctx, tx, w.repliedTaskInput(lead, arrival, title, owner))
}

// FileTx is the contacts store's LeadArrivals port: the task for the owner of
// a contact a lead arrived for. It is keyed on the arrival, so a resent
// submission files one task.
func (w leadWorkTasks) FileTx(ctx context.Context, tx pgx.Tx, arrival contacts.LeadArrival) error {
	name := arrival.Fields.FullName
	if name == "" {
		name = arrival.Fields.Email
	}
	title := fmt.Sprintf(leadForContactSubject, name)
	return w.logSystemTaskTx(ctx, tx, w.arrivalTaskInput(arrival, title, leadArrivalBody(arrival)))
}

// leadArrivalBody lists what arrived, where from, and what to do with it.
func leadArrivalBody(arrival contacts.LeadArrival) string {
	var b strings.Builder
	for _, field := range []struct{ label, value string }{
		{"Name", arrival.Fields.FullName},
		{"Email", arrival.Fields.Email},
		{"Phone", arrival.Fields.Phone},
		{"Title", arrival.Fields.Title},
		{"Company", arrival.Fields.CompanyName},
		{"LinkedIn", arrival.Fields.LinkedInURL},
	} {
		if field.value != "" {
			fmt.Fprintf(&b, "%s: %s\n", field.label, field.value)
		}
	}
	fmt.Fprintf(&b, "Came from: %s\n\n", arrival.Origin)
	b.WriteString("No lead was created. If this is a new opportunity, open a deal with the contact.")
	return b.String()
}

// repliedTaskInput is the ask to qualify a lead who wrote or booked.
func (w leadWorkTasks) repliedTaskInput(lead ids.LeadID, arrival ids.UUID, title string, owner *ids.UUID) activities.LogActivityInput {
	at := w.now().UTC()
	source, key := provenance.LeadRepliedSource, arrival.String()
	return activities.LogActivityInput{
		Kind: activityKindTask, Subject: &title, OccurredAt: &at,
		SourceSystem: &source, SourceID: &key, SourceActivityID: &arrival,
		Links:      []activities.ActivityLinkInput{{EntityType: entityLead, EntityID: lead.UUID}},
		Source:     systemActor,
		Origin:     activities.OriginSystemRemediation,
		AssigneeID: ownerAsAssignee(owner),
	}
}

// arrivalTaskInput tells a contact's owner about a lead that arrived for them.
func (w leadWorkTasks) arrivalTaskInput(arrival contacts.LeadArrival, title, body string) activities.LogActivityInput {
	at := w.now().UTC()
	source, key := provenance.LeadForContactSource, arrival.Key
	return activities.LogActivityInput{
		Kind: activityKindTask, Subject: &title, Body: &body, OccurredAt: &at,
		SourceSystem: &source, SourceID: &key,
		Links:      []activities.ActivityLinkInput{{EntityType: entityContact, EntityID: arrival.ContactID.UUID}},
		Source:     systemActor,
		Origin:     activities.OriginSystemRemediation,
		AssigneeID: ownerAsAssignee(&arrival.OwnerID),
	}
}

func ownerAsAssignee(owner *ids.UUID) *ids.UserID {
	if owner == nil {
		return nil
	}
	user := ids.From[ids.UserKind](*owner)
	return &user
}

// logSystemTaskTx writes the task. An owner who can no longer hold work costs
// the task its assignee, not its existence, as logQualifyReminder rules. The
// first try runs under a savepoint, so its refusal leaves tx usable.
func (w leadWorkTasks) logSystemTaskTx(ctx context.Context, tx pgx.Tx, in activities.LogActivityInput) error {
	store := activities.NewStore(w.db)
	attempt, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("compose: opening the task savepoint: %w", err)
	}
	if _, _, err = store.LogActivityTx(ctx, attempt, in); err == nil {
		return attempt.Commit(ctx)
	}
	if rbErr := attempt.Rollback(ctx); rbErr != nil {
		return errors.Join(err, rbErr)
	}
	if in.AssigneeID == nil || !unassignableSeat(err) {
		return err
	}
	in.AssigneeID = nil
	_, _, err = store.LogActivityTx(ctx, tx, in)
	return err
}

// contactArrivalTeller tells a contact's owner when capture refused a lead for
// that contact.
type contactArrivalTeller struct {
	store *contacts.Store
	log   *slog.Logger
}

// TellContactOwner is logged and never returned: the capture it follows has
// committed, and the record it was about is already the contact.
//
// A name read off a company's website is not a submission, and its accept
// keeps nothing for a contact (siteleadaccept.go), so it tells nobody.
func (t contactArrivalTeller) TellContactOwner(ctx context.Context, contact ids.ContactID, rec connector.NormalizedRecord, fields capture.LeadFields) {
	if rec.NaturalKey.SourceSystem == provenance.WebsiteLeadSource {
		return
	}
	in := contacts.CreateLeadInput{
		FullName: &fields.FullName, Email: &fields.Email, CompanyName: &fields.CompanyName, Title: &fields.Title,
		SourceSystem: &rec.NaturalKey.SourceSystem, SourceID: &rec.NaturalKey.SourceID,
	}
	origin := "the " + rec.NaturalKey.SourceSystem + " connector"
	if _, err := t.store.TellContactOwnerOfArrival(ctx, contact, in, origin); err != nil {
		t.log.ErrorContext(ctx, "capture: telling a contact's owner about a captured lead failed",
			"contact", contact.String(), "err", err)
	}
}
