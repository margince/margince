// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestAReviewedOldInquiryRunsOnceAfterItsEarlierUnmatchedEvent(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Inquiries", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"old@visitor.example"}, "message": {"A private old question"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("submission: %d %s", answer.Status, answer.Body)
	}
	original := inquiryEvent(t, e, "old@visitor.example", "activity.captured")
	policy := setupInquiryPolicy(t, e, form, "one_colleague")
	engine := compose.NewWorkflowEngine(e.DB())
	if err := engine.HandleEvent(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	var runs int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM workflow_run WHERE handler=@handler`, pgx.NamedArgs{"handler": provenance.WebsiteInquirySource}).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("unreviewed old source claimed %d runs", runs)
	}
	if policy.OwnerId == nil {
		t.Fatal("saved policy has no authorizer")
	}
	ctx := inquirySeatContext(t, e, ids.UUID(*policy.OwnerId))
	store := activities.NewStore(e.DB())
	noteID := ids.From[ids.ActivityKind](original.Entity.ID)
	before, err := store.GetActivityContent(ctx, noteID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if before.Version == nil {
		t.Fatal("source has no version")
	}
	adopted, err := store.AdoptWebsiteInquiry(ctx, noteID, int64(*before.Version), mustParseForm(t, form.ID), ids.UUID(policy.Id))
	if err != nil {
		t.Fatal(err)
	}
	if adopted.Activity.Version == nil || *adopted.Activity.Version <= *before.Version || adopted.Event.Type != "activity.updated" || adopted.Event.EventID == ids.Nil {
		t.Fatalf("review did not commit a new source version and event: %+v", adopted)
	}
	owner, err := workflow.AcceptedInquiryOwner(adopted.Activity.Raw)
	if err != nil || owner == nil || *owner != ids.UUID(policy.Id) {
		t.Fatalf("reviewed owner: %v %v", owner, err)
	}
	if _, err := store.AdoptWebsiteInquiry(ctx, noteID, int64(*before.Version), mustParseForm(t, form.ID), ids.UUID(policy.Id)); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale review: %v", err)
	}
	if _, err := store.AdoptWebsiteInquiry(ctx, noteID, int64(*adopted.Activity.Version), mustParseForm(t, form.ID), ids.NewV7()); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("competing policy replaced occurrence ownership: %v", err)
	}
	var audit string
	if err := e.Owner.QueryRow(t.Context(), `SELECT after::text FROM audit_log WHERE id=@id`, pgx.NamedArgs{"id": adopted.Event.Trace.AuditLogID}).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	for _, metadata := range []string{audit, string(adopted.Event.Payload)} {
		if strings.Contains(metadata, "private old question") || strings.Contains(metadata, "old@visitor.example") {
			t.Fatalf("handling metadata copied submission content: %s", metadata)
		}
	}
	for _, event := range []kevents.Envelope{adopted.Event, adopted.Event, original} {
		if err := engine.HandleEvent(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}

	var tasks int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM activity WHERE kind='task' AND source_system=@source AND source_activity_id=@note`, pgx.NamedArgs{"source": provenance.WebsiteInquirySource, "note": noteID.UUID}).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 {
		t.Fatalf("review and event replays created %d inquiry tasks", tasks)
	}
}

func mustParseForm(t *testing.T, raw string) ids.UUID {
	t.Helper()
	id, err := ids.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
