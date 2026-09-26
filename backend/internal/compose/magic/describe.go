// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Saying what a machine write DID, who did it, and why — or not showing it.
//
// "A record was updated" was the sentence for every `update` the ledger holds,
// and a background job writes one per record it touches: a mailbox backfill put
// 1,200 identical lines on the page, each naming no record and saying nothing a
// reader could act on. An update is shown only when this file can say what it
// changed; the rest is housekeeping, counted in not_shown.

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// description is what one audit row says to a reader.
type description struct {
	summary crmcontracts.MagicSentence
	// reason is why the machinery did it, where the row records a why.
	reason *crmcontracts.MagicSentence
}

// cohortKeys are the counters the contacts module audits when it files
// captured mail under a contact (contacts.PromoteContactCohortTx).
var cohortKeys = []string{"cohort_linked", "cohort_promoted"}

// bookkeepingFields are after-image keys that record the machinery's own state
// rather than anything about the record. A change touching only these tells a
// reader nothing.
var bookkeepingFields = map[string]bool{
	"updated_at": true, "version": true, "chunk_count": true, "checksum": true,
	"audience_reason": true, "owed_verdict_ruleset": true, "reply_verdict_by": true,
}

// describe answers what an admitted audit row means to a reader, and whether it
// means anything at all.
func describe(e entry) (description, bool) {
	if e.Action != "update" {
		meaning, ok := meaningOf(e.Action)
		if !ok {
			return description{}, false
		}
		return description{summary: crmcontracts.MagicSentence{Key: meaning.sentence}}, true
	}
	after := objectOf(e.After)
	if hasAny(after, cohortKeys) {
		return description{
			summary: crmcontracts.MagicSentence{Key: "magic.action.mail_filed"},
			reason:  &crmcontracts.MagicSentence{Key: "magic.why.mail_filed"},
		}, true
	}
	evidence := objectOf(e.Evidence)
	if facts, ok := evidence["facts"].([]any); ok && len(facts) > 0 && len(changedFields(objectOf(e.Before), after)) == 0 {
		return description{
			summary: crmcontracts.MagicSentence{Key: "magic.action.company_profile_read"},
			reason:  &crmcontracts.MagicSentence{Key: "magic.why.public_records"},
		}, true
	}
	fields := changedFields(objectOf(e.Before), after)
	if len(fields) == 0 {
		return description{}, false
	}
	values := map[string]string{"fields": strings.Join(fields, ", ")}
	return description{
		summary: crmcontracts.MagicSentence{Key: "magic.action.fields_changed", Values: &values},
		reason:  reasonFromEvidence(evidence),
	}, true
}

// changedFields lists, readably and sorted, the fields an update moved: every
// after-image key that is not bookkeeping and whose value differs from before.
func changedFields(before, after map[string]any) []string {
	var out []string
	for k, v := range after {
		if bookkeepingFields[k] {
			continue
		}
		if b, had := before[k]; had && equalJSON(b, v) {
			continue
		}
		out = append(out, strings.ReplaceAll(k, "_", " "))
	}
	sort.Strings(out)
	return out
}

// reasonFromEvidence turns the source an enrichment recorded into a why.
func reasonFromEvidence(evidence map[string]any) *crmcontracts.MagicSentence {
	switch evidence["source"] {
	case "site_read":
		ref, _ := evidence["source_ref"].(string)
		values := map[string]string{"site": siteOf(ref)}
		return &crmcontracts.MagicSentence{Key: "magic.why.site_read", Values: &values}
	case "capture_enrich":
		return &crmcontracts.MagicSentence{Key: "magic.why.signature"}
	}
	return nil
}

// siteOf reduces a recorded source (`site_read:https://host/path`) to its host.
func siteOf(ref string) string {
	raw := strings.TrimPrefix(ref, "site_read:")
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return raw
}

// actorLabel names who acted, as a reader would call them.
//
// The ledger's actor ids are internal names (`link-reconcile`,
// `system:technical-lookup`); the page shows what the job IS. An id this map
// does not know falls back to its kind — an agent, the system, a mailbox —
// which is less than a name and still more than nothing.
func actorLabel(e entry) crmcontracts.MagicSentence {
	if isRetention(e) {
		return crmcontracts.MagicSentence{Key: "magic.by.retention"}
	}
	if key, ok := actorKeys[e.ActorID]; ok {
		return crmcontracts.MagicSentence{Key: key}
	}
	switch {
	case strings.HasPrefix(e.ActorID, "connector:"):
		return crmcontracts.MagicSentence{Key: "magic.by.mailbox"}
	case strings.HasPrefix(e.ActorID, "automation:"):
		return crmcontracts.MagicSentence{Key: "magic.by.automation"}
	case e.ActorType == string(crmcontracts.MagicActorTypeMagicActorAgent):
		return crmcontracts.MagicSentence{Key: "magic.by.agent"}
	}
	return crmcontracts.MagicSentence{Key: "magic.by.system"}
}

// actorKeys are the background jobs a reader meets on this page, by the actor
// id their audit rows carry.
var actorKeys = map[string]string{
	"link-reconcile":             "magic.by.mail_filing",
	"cohort-promote":             "magic.by.mail_filing",
	"system:technical-lookup":    "magic.by.company_lookup",
	"agent:deepread":             "magic.by.website_reader",
	"system:contact_auto_enrich": "magic.by.website_reader",
	"system:capture_auto_enrich": "magic.by.website_reader",
	"agent:enrich":               "magic.by.signature_reader",
	"agent:overnight":            "magic.by.overnight_agent",
	"system:capture_classify":    "magic.by.mail_reader",
	"system:owed_verdict":        "magic.by.mail_reader",
	"agent:signal-scan":          "magic.by.mail_reader",
	"system:auto_apply":          "magic.by.auto_apply",
	"system:automation":          "magic.by.automation",
	"system:lead-router":         "magic.by.lead_routing",
	"system:audience_rescope":    "magic.by.system",
	"system:forecast-snapshot":   "magic.by.system",
	"system:notice-case-open":    "magic.by.system",
	"system:handbook-corpus":     "magic.by.system",
	"agent:knowledge-ingest":     "magic.by.system",
	"routing-seed":               "magic.by.system",
}

// isRetention reports an audit row the retention engine wrote.
func isRetention(e entry) bool {
	_, ok := objectOf(e.Evidence)["retention_action"]
	return ok
}

func objectOf(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func hasAny(m map[string]any, keys []string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func equalJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}
