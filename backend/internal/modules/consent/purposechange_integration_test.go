// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func purposeConfigurer(e *channelConsentEnv) context.Context {
	return principal.WithActor(e.ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"consent_config": {Create: true, Update: true}, "contact": {Read: true, Update: true}}, RowScope: principal.RowScopeAll}})
}

func TestAnUnusedPurposeCanChangeWithItsAuditAndEvent(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := purposeConfigurer(e)
	created, err := e.store.CreatePurpose(ctx, "spring_news", "Spring News", false, "")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := e.store.RenamePurpose(ctx, created.ID, "Spring newsletter")
	if err != nil || renamed.Label != "Spring newsletter" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	if err := e.store.ArchivePurpose(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	purposes, err := e.store.ListPurposes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range purposes {
		if p.ID == created.ID {
			t.Fatal("archived purpose offered for new consent")
		}
	}
	var audits, events int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type='consent_purpose' AND entity_id=$1`, created.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1::uuid::text AND envelope->>'type'='consent_purpose.changed'`, created.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 3 || events != 3 {
		t.Fatalf("audits=%d events=%d, want one per create/rename/archive", audits, events)
	}
	wording := "Newsletter"
	if _, err := e.store.Record(ctx, RecordInput{ContactID: e.contact, PurposeID: created.ID, NewState: "granted", PolicyText: &wording}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("archived grant: %v", err)
	}
}

func TestARenameCannotBeMistakenForAnotherListedPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := purposeConfigurer(e)
	if _, err := e.store.CreatePurpose(ctx, "partner_news", "Partner news", false, ""); err != nil {
		t.Fatal(err)
	}
	spring, err := e.store.CreatePurpose(ctx, "spring_news", "Spring news", false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"PARTNER   news", "partner_news"} {
		var refused *ValidationError
		if _, err := e.store.RenamePurpose(ctx, spring.ID, label); !errors.As(err, &refused) || refused.Field != "label" {
			t.Fatalf("RenamePurpose(%q) = %v, want a refusal on label", label, err)
		}
	}
	renamed, err := e.store.RenamePurpose(ctx, spring.ID, "Spring_News")
	if err != nil || renamed.Label != "Spring_News" {
		t.Fatalf("a rename to the purpose's own key: %+v %v", renamed, err)
	}
}

func TestConsentAndWordingReferencesPreventPurposeChanges(t *testing.T) {
	for _, reference := range []string{"grant", "lead state", "text id", "text key", "decision", "scheduled legacy", "scheduled marketing", "scheduled spaced", "automation action", "automation params", "staged draft"} {
		t.Run(reference, func(t *testing.T) {
			e := setupChannelConsent(t)
			ctx := purposeConfigurer(e)
			created, err := e.store.CreatePurpose(ctx, "purpose_held", "Purpose held", false, "")
			if err != nil {
				t.Fatal(err)
			}
			switch reference {
			case "grant":
				wording := "Newsletter"
				_, err = e.store.Record(ctx, RecordInput{ContactID: e.contact, PurposeID: created.ID, NewState: "granted", PolicyText: &wording})
			case "lead state":
				lead := ids.NewV7()
				if _, err = e.owner.Exec(ctx, `INSERT INTO lead(id,source,captured_by) VALUES($1,'manual','human:test')`, lead); err == nil {
					_, err = e.owner.Exec(ctx, `INSERT INTO contact_consent(lead_id,purpose_id,state,captured_at,source) VALUES($1,$2,'granted',now(),'form')`, lead, created.ID)
				}
			case "scheduled legacy", "scheduled marketing", "scheduled spaced":
				key := "consent_purpose"
				if reference == "scheduled marketing" {
					key = "marketing_purpose"
				}
				value := created.Key
				if reference == "scheduled spaced" {
					value = "\t  PURPOSE_HELD\u00a0"
				}
				_, err = e.owner.Exec(ctx, `INSERT INTO scheduled_send(scheduled_by,scheduled_at,scheduled_tz,origin_kind,origin_links,principal_kind,payload) VALUES($1,now(),'UTC','account','[]','human',jsonb_build_object($2::text,$3::text))`, e.user, key, value)
			case "text id":
				_, err = e.owner.Exec(ctx, `INSERT INTO consent_text_version (key,body,version,purpose_id) VALUES ('wording','Shown wording','v1',$1)`, created.ID)
			case "text key":
				_, err = e.owner.Exec(ctx, `INSERT INTO consent_text_version (key,body,version) VALUES ($1,'Shown wording','v1')`, created.Key)
			case "automation action", "automation params":
				action, params := `{"type":"draft_email"}`, `{}`
				configured := `{"nested":{"consent_purpose":" PURPOSE_HELD "}}`
				if reference == "automation action" {
					action = configured
				} else {
					params = configured
				}
				_, err = e.owner.Exec(ctx, `INSERT INTO automation(key,name,trigger,action,params) VALUES('purpose_reference','Purpose reference','{}',$1,$2)`, action, params)
			case "staged draft":
				_, err = e.owner.Exec(ctx, `INSERT INTO approval(kind,proposed_change,diff_hash,proposed_by,expires_at,requested_decider_id,requested_decider_source) VALUES('send_email',jsonb_build_object('message',jsonb_build_object('marketing_purpose',$1::text)),repeat('a',64),'human:test',now()+interval '1 day',$2,'stager')`, created.Key, e.user)
			case "decision":
				delivery := plantConsentDelivery(t, e.owner, e.user)
				_, err = e.owner.Exec(ctx, `INSERT INTO communication_decision (delivery_id,attempt,decision_set_id,recipient_address,phase,resolved_category,verdict,reason_code,mode,actor,purpose_key) VALUES ($1,0,$2,'reader@example.test','staging','marketing','review','no_compatible_evidence','observe','test',$3)`, delivery, ids.NewV7(), created.Key)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, change := range []func() error{func() error { _, err := e.store.RenamePurpose(ctx, created.ID, "Changed label"); return err }, func() error { return e.store.ArchivePurpose(ctx, created.ID) }} {
				var used *PurposeInUseError
				if err := change(); !errors.As(err, &used) {
					t.Fatalf("%s did not protect its purpose: %v", reference, err)
				}
			}
		})
	}
}

func TestEveryPurposeForeignKeyRequiresALivePurpose(t *testing.T) {
	e := setupChannelConsent(t)
	var missing int
	if err := e.owner.QueryRow(e.ctx, `SELECT count(*) FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid='public.consent_purpose'::regclass AND NOT EXISTS (SELECT 1 FROM pg_trigger tr WHERE tr.tgrelid=fk.conrelid AND tr.tgname='live_consent_purpose_reference')`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Fatalf("%d purpose foreign keys lack the live-purpose guard", missing)
	}
}
