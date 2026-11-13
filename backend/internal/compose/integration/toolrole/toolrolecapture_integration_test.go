// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sweepMailbox is the provider of the mailbox toolRoleApp connects.
const sweepMailbox = "gmail"

func uniqueDomain() string { return "sweep-" + ids.NewV7().String()[24:] + ".test" }

// captureFixtures fill the capture and connector calls: the user's lists, the
// workspace's domain decisions and the settings of a connected mailbox.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var captureFixtures = map[string]func(p *recordPool) map[string]any{
	"add_consumer_mail_domain": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"domain": uniqueDomain(), "kind": "extra"}}
	},
	"remove_consumer_mail_domain": byID("consumer_mail_domain"),
	"add_counterparty_hold": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"kind": "domain", "value": uniqueDomain()}}
	},
	"lift_counterparty_hold": byID("agent_counterparty_hold"),
	"add_capture_exclusion": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"scope": "user", "kind": "domain", "value": uniqueDomain()}}
	},
	"lift_capture_exclusion": byID("agent_capture_exclusion"),
	"add_owner_identity": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"kind": "address", "value": uniqueEmail()}}
	},
	"withdraw_owner_identity": byID("owner_identity"),
	"restore_owner_identity":  byID("agent_withdrawn_owner_identity"),
	"add_workspace_email_domain": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"domain": uniqueDomain()}}
	},
	"remove_workspace_email_domain": onDomain("workspace_email_domain"),
	"discard_domain_question":       onDomain("domain_question"),
	"keep_domain_as_company":        onDomain("domain_question"),
	"keep_domain_as_vendor":         onDomain("vendor_question"),
	"reopen_withheld_domain":        onDomain("withheld_domain"),
	"set_blocked_domain": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"domain": uniqueDomain(), "admission": "suppressed", "reason": "Role sweep."}}
	},
	"set_capture_sender_decision": func(*recordPool) map[string]any {
		return map[string]any{"address": uniqueEmail(), "request_body": map[string]any{"decision": "business"}}
	},
	"withdraw_capture_sender_decision": func(p *recordPool) map[string]any {
		return map[string]any{"address": p.take("agent_sender_decision")}
	},
	"set_connector_context_tag": func(p *recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"tag_id": p.take("tag")}}
	},
	"set_connector_history_baseline": func(p *recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"date": pastDay(p), "timezone": "UTC"}}
	},
	"set_connector_mail_posture": func(*recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"posture": "classified"}}
	},
	"set_connector_signature_enrichment": func(*recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"enabled": false}}
	},
	"dismiss_connector_silence": func(p *recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"since": p.take("silent_mailbox")}}
	},
	"preview_connector_backfill": func(*recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"window": "3m"}}
	},
	"start_connector_backfill": func(*recordPool) map[string]any {
		return map[string]any{"provider": sweepMailbox, "request_body": map[string]any{"window": "3m"}}
	},
	"cancel_connector_backfill": func(p *recordPool) map[string]any { return map[string]any{"provider": p.take("running_backfill")} },
	"disconnect_channel":        byID("live_channel"),
	"update_import_run": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("draft_import_run"), "request_body": map[string]any{"version": 1, "draft_step": "map"}}
	},
	"validate_import_run": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("draft_import_run"), "request_body": map[string]any{"version": 1}}
	},
	"cancel_import_run": byID("draft_import_run"),
	"undo_import_run":   byID("complete_import_run"),
}

//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func onDomain(kind string) func(*recordPool) map[string]any {
	return func(p *recordPool) map[string]any { return map[string]any{"domain": p.take(kind)} }
}

// captureSeeders make the capture lists, domain decisions and import runs the
// calls above take.
func captureSeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"consumer_mail_domain": func() string {
			return s.post("/v1/capture/consumer-mail-domains", integration.AnyMap{"domain": uniqueDomain(), "kind": "extra"})
		},
		// An agent lifts only what an agent added, so each is added as the call's own
		// passport, as it is taken.
		"agent_counterparty_hold": func() string {
			return s.asCallerWhereItMay("POST", "/v1/capture/counterparty-holds", integration.AnyMap{"kind": "domain", "value": uniqueDomain()})
		},
		"agent_capture_exclusion": func() string {
			return s.asCallerWhereItMay("POST", "/v1/capture/exclusions", integration.AnyMap{"scope": "user", "kind": "domain", "value": uniqueDomain()})
		},
		"owner_identity": func() string {
			return s.post("/v1/capture/owner-identities", integration.AnyMap{"kind": "address", "value": uniqueEmail()})
		},
		"workspace_email_domain": func() string {
			domain := uniqueDomain()
			s.call("POST", "/v1/capture/email-domains", integration.AnyMap{"domain": domain}, nil, nil)
			return domain
		},
		"domain_question": func() string { return openDomainQuestion(s, "unevidenced") },
		"vendor_question": func() string { return openDomainQuestion(s, "vendor_verdict") },
		// A domain the triage sweep set aside with a reason, which a user may ask about again.
		"withheld_domain": func() string {
			domain := uniqueDomain()
			s.ownerExec(`INSERT INTO company_domain_disposition (domain, status, source, pending_reason) VALUES ($1, 'pending', NULL, 'unevidenced')
				ON CONFLICT (domain) DO UPDATE SET status = 'pending', source = NULL, pending_reason = 'unevidenced'`, domain)
			return domain
		},
		// The decision the agent withdraws is an agent's: one its user made stays
		// the user's. Written as the decision route writes it, so a role refused
		// the decision itself still has one to withdraw.
		"agent_sender_decision": func() string {
			address := uniqueEmail()
			s.ownerExec(`INSERT INTO capture_sender_override (user_id, address, decision, captured_by) VALUES ($1, $2, 'business', 'agent:role-sweep')`,
				apptest.SessionUserID(s.t, s.e), address)
			return address
		},
		// Only the one who withdrew an identity restores it, so it is withdrawn
		// as the call's own passport, as it is taken.
		"agent_withdrawn_owner_identity": func() string {
			var identity struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/capture/owner-identities", integration.AnyMap{"kind": "address", "value": uniqueEmail()}, s.pool.caller, &identity)
			s.call("DELETE", "/v1/capture/owner-identities/"+identity.ID, nil, s.pool.caller, nil)
			return identity.ID
		},
		// The one Telegram bot a workspace binds, connected again where a
		// disconnect earlier in the sweep took it off.
		// A backfill to cancel. One the sweep's own start left running stands.
		"running_backfill": func() string {
			s.e.Call(s.t, "POST", "/v1/connectors/"+sweepMailbox+"/backfill", integration.AnyMap{"window": "3m"}, nil, nil)
			return sweepMailbox
		},
		"silent_mailbox": func() string {
			now := clocktest.Now(s.t)
			s.ownerExec(`UPDATE capture_connection SET connected_at = $3, silence_dismissed_since = NULL
				WHERE user_id = $1 AND provider = $2`, apptest.SessionUserID(s.t, s.e), sweepMailbox, now.Add(-10*24*time.Hour))
			s.ownerExec(`INSERT INTO capture_sync_state (connection_id, last_success_at)
				SELECT id, $3 FROM capture_connection WHERE user_id = $1 AND provider = $2
				ON CONFLICT (connection_id) DO UPDATE SET last_success_at = $3, last_error_class = NULL, last_delivery_at = NULL`,
				apptest.SessionUserID(s.t, s.e), sweepMailbox, now)
			return s.ownerScalar(`SELECT to_char(connected_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
				FROM capture_connection WHERE user_id = $1 AND provider = $2`, apptest.SessionUserID(s.t, s.e), sweepMailbox)
		},
		"live_channel": func() string {
			c := &apptest.ChannelConversation{AppEnv: s.e, User: apptest.SessionUserID(s.t, s.e)}
			if s.ownerScalar(`SELECT count(*)::text FROM channel_connection WHERE provider = 'telegram' AND archived_at IS NULL`) == "0" {
				c.ConnectBot(s.t)
			}
			return s.ownerScalar(`SELECT id::text FROM channel_connection WHERE provider = 'telegram' AND archived_at IS NULL`)
		},
		"draft_import_run": s.alone(func() string {
			profile, status := importjobtest.UploadCSV(s.t, s.e, "company", "display_name\nSweep "+ids.NewV7().String()+"\n")
			if status != http.StatusOK || profile.RunID == "" {
				s.t.Fatalf("uploading an import's file → %d %+v", status, profile)
			}
			return profile.RunID
		}),
		// The run is tested, approved, and its commit job worked, as the
		// worker leaves it complete.
		"complete_import_run": s.alone(func() string {
			profile, status := importjobtest.UploadCSV(s.t, s.e, "company", "display_name\nSweep "+ids.NewV7().String()+"\n")
			if status != http.StatusOK {
				s.t.Fatalf("uploading an import's file → %d", status)
			}
			run, status := importjobtest.CreateRunWithMapping(s.t, s.e, "company", profile.SourceRef, map[string]string{"display_name": "display_name"})
			if status != http.StatusAccepted && status != http.StatusCreated {
				s.t.Fatalf("starting the import → %d", status)
			}
			importjobtest.Drain(s.t, s.e)
			s.call("POST", "/v1/imports/"+run.ID+"/approve", nil, nil, nil)
			importjobtest.Drain(s.t, s.e)
			return run.ID
		}),
	}
}

// openDomainQuestion asks the sweep's user about a new sender's domain, for a
// pending reason. No route asks one: capture asks the sole seat a new sender's
// mail is addressed to, which these rows stand for.
func openDomainQuestion(s sweepSeed, reason string) string {
	domain := uniqueDomain()
	sender := "contact@" + domain
	contact := s.post("/v1/contacts", integration.AnyMap{"full_name": "Role Sweep", "source": "manual", "emails": []integration.AnyMap{{"email": sender}}})
	mail := s.post("/v1/activities", integration.AnyMap{
		"kind": "email", "direction": "inbound", "subject": "Role sweep", "body": "Role sweep", "source": "manual",
		"links": []integration.AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})
	// The question is current: its mail came an hour ago.
	s.ownerExec(`UPDATE activity SET occurred_at = $2 WHERE id = $1`, mail, clocktest.Now(s.t).Add(-time.Hour))
	s.ownerExec(`INSERT INTO activity_participant (activity_id, address, address_domain_folded, role) VALUES ($1, $2, $3, 'from')`, mail, sender, domain)
	s.ownerExec(`INSERT INTO activity_participant (activity_id, address, role)
		SELECT $1, email, 'to' FROM app_user WHERE id = $2`, mail, apptest.SessionUserID(s.t, s.e))
	s.ownerExec(`INSERT INTO company_domain_disposition (domain, status, source, pending_reason) VALUES ($1, 'pending', NULL, $2)
		ON CONFLICT (domain) DO UPDATE SET status = 'pending', source = NULL, pending_reason = $2`, domain, reason)
	return domain
}

// sweepGmail answers the Gmail API from memory: the labels a folder list reads
// and the count a backfill preview reads. Any other host reaches next.
type sweepGmail struct{ next http.RoundTripper }

func (g sweepGmail) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "gmail.googleapis.com" {
		return g.next.RoundTrip(r)
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/labels"):
		return apptest.BookingProviderResponse(r, `{"labels":[{"id":"INBOX","name":"INBOX","type":"system"}]}`), nil
	case strings.HasSuffix(r.URL.Path, "/messages"):
		return apptest.BookingProviderResponse(r, `{"messages":[],"resultSizeEstimate":12}`), nil
	}
	return nil, fmt.Errorf("unexpected Gmail fixture request: %s %s", r.Method, r.URL.Path)
}
