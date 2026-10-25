// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every activity time on the wire says how precisely it is known.
//
// A day entry is stored at noon UTC of its date. A projection that hands out
// that instant with no precision beside it reads as "12:00" to every client.
// The frontend's precision gate cannot see a field the contract never declared.
//
// So every `*_at` a contract declares is judged. It is precise when its
// `<x>_precision` sits beside it, or an `occurred_precision` that says it reads
// that time. One its name or description calls an activity's time must be
// precise, or name in precisionlessTimes why no day entry reaches it. Any other
// is waived by its name in instantTimeNames, which says why it is not an
// activity's time, or in precisionlessTimes. A time no list places is a finding.

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// precisionContracts are the specs whose schemas reach a client.
var precisionContracts = []string{"api/crm.yaml", "api/public-events.yaml"}

var (
	// activityTimeDescribed is a description that calls a `<x>_at` an
	// activity's time, or names the `occurred_at` it is taken from.
	activityTimeDescribed = regexp.MustCompile("(?i)\\b(?:activity|activities)'?s? time\\b|`occurred_at`")
	// activityTimeNamed is a name that says the time is when something was
	// last done with someone: a touch, a mail either way, a meeting.
	activityTimeNamed = regexp.MustCompile(`^(?:occurred_at|last_\w+|\w*(?:activity|activities|inbound|outbound|meeting|contact|interaction|on_thread)\w*)$`)
)

// minWireTimes is the floor: every `*_at` the two contracts declared when the
// census was last counted. Fewer means the walk reads less than it should.
const minWireTimes = 597

// storedClock is the reason a stored clock carries no precision.
const storedClock = "stored clock without a precision column, issue #8910"

// precisionlessTimes are the times with no precision that a name rule does not
// place, each with the reason. Keyed "<spec> <schema path>.<property>" as the
// census prints it.
var precisionlessTimes = gatekit.Waive(map[string]string{
	"api/crm.yaml AgentClient.last_used_at":                  "when an agent client last called Margince, a request's own instant",
	"api/crm.yaml AiCallAttempt.occurred_at":                 "when a model call was made, an ai_call row's own instant",
	"api/crm.yaml AiCallSummary.occurred_at":                 "when a model call was made, an ai_call row's own instant",
	"api/crm.yaml AiRunModelUsage.last_used_at":              "when a model was last called, an ai_call row's own instant",
	"api/crm.yaml AiRungHealth.last_call_at":                 "when a model tier last answered, an ai_call row's own instant",
	"api/crm.yaml AuditHistoryEntry.occurred_at":             "when an audit row was written, the write's own instant",
	"api/crm.yaml AuditLogEntry.occurred_at":                 "when an audit row was written, the write's own instant",
	"api/crm.yaml Automation.last_run_at":                    "when an automation last ran, the run's own instant",
	"api/crm.yaml AutomationRun.occurred_at":                 "when an automation ran, the run's own instant",
	"api/crm.yaml CaptureConnection.last_synced_at":          "when a mailbox sync last ran, the sync's own instant",
	"api/crm.yaml CaptureSweepHealth.last_succeeded_at":      "when a capture pass last finished, the pass's own instant",
	"api/crm.yaml CaptureTraceEntry.occurred_at":             "when capture logged a step, the trace row's own instant",
	"api/crm.yaml ClaimEvidence.last_verified_at":            "when a human last confirmed a claim, that act's own instant",
	"api/crm.yaml CompanyFinanceSummary.last_synced_at":      "when a finance sync last finished, the sync's own instant",
	"api/crm.yaml ConsentEvent.occurred_at":                  "when consent changed, the ledger row's own instant",
	"api/crm.yaml DataTaskProgress.last_progress_at":         "when a data task last moved, the task's own instant",
	"api/crm.yaml DealRoomParticipant.last_seen_at":          "when a buyer last made a request to the deal room, a request's own instant",
	"api/crm.yaml ExtensionIngestRefusal.last_refused_at":    "when an extension's write was last refused, the refusal's own instant",
	"api/crm.yaml ExtensionUnitIngestHealth.last_refused_at": "when an extension's write was last refused, the refusal's own instant",
	"api/crm.yaml FinanceConnection.last_attempt_at":         "when a finance sync was last tried, the attempt's own instant",
	"api/crm.yaml FinanceConnection.last_success_at":         "when a finance sync last succeeded, the sync's own instant",
	"api/crm.yaml InputCheck.last_seen_at":                   "when a check run last found the problem, the run's own instant",
	"api/crm.yaml MorningDigest.connectors.last_synced_at":   "when a connector last synced, the sync's own instant",
	"api/crm.yaml MySession.last_active_at":                  "when a sign-in session last made a request, a request's own instant",
	"api/crm.yaml PassportSummary.last_used_at":              "when an agent passport was last used, a request's own instant",
	"api/crm.yaml ProviderConnection.last_used_at":           "when a provider key was last used, a call's own instant",
	"api/crm.yaml ProviderConnection.last_verified_at":       "when a provider key was last checked, the check's own instant",
	"api/crm.yaml RecordViewAck.last_viewed_at":              "when a reader last acknowledged a record, a request's own instant",
	"api/crm.yaml ReportingCaptureStatus.last_attempt_at":    "when a report capture was last tried, the attempt's own instant",
	"api/crm.yaml ReportingCaptureStatus.last_success_at":    "when a report capture last succeeded, the capture's own instant",
	"api/crm.yaml TechnicalEnrichLane.last_success_at":       "when an enrichment source last answered, the call's own instant",
	"api/crm.yaml User.last_active_at":                       "when a member last made a request, a request's own instant",
	"api/crm.yaml UserSession.last_active_at":                "when a sign-in session last made a request, a request's own instant",
	"api/crm.yaml VoiceProfile.last_built_at":                "when a voice profile was last built, the build's own instant",
	"api/crm.yaml ConsentQualifyingEvent.occurred_at": "the proof a consent guard holds in consent_qualifying_event; no client draws it, " +
		"and the guard measures it against windows of 90 days and more, which a date's hours do not move",
	"api/crm.yaml QualifyingEventRecord.occurred_at":             "a qualifying exchange a rep stated, stored in consent_qualifying_event, not an activity",
	"api/crm.yaml RecordQualifyingEventRequest.occurred_at":      "a qualifying exchange a rep states, written to consent_qualifying_event, not an activity",
	"api/crm.yaml RecordCommunicationContextRequest.occurred_at": "a rep's statement about a contact, written to consent_qualifying_event, not an activity",
	"api/crm.yaml NoticeAcquisition.occurred_at": "when a contact's data was acquired, stated by a rep or taken from captured mail's sent " +
		"instant, held in contact_acquisition_evidence and not an activity",
	"api/crm.yaml NoticeOrigin.occurred_at":                         "when the event behind a notification was emitted, the bus's own instant",
	"api/crm.yaml ListDependency.occurred_at":                       "when an export ran or a list rule was made, not an activity",
	"api/crm.yaml ListHistoryEntry.occurred_at":                     "when a list membership or revision changed, not an activity",
	"api/crm.yaml MagicLine.occurred_at":                            "when an audited change, decision or run happened, each its own instant",
	"api/crm.yaml MagicLineRecord.occurred_at":                      "when an audited change was written, the audit row's own instant",
	"api/crm.yaml MorningDigest.projects.phase_changes.occurred_at": "when a project changed phase, a history row's own instant",
	"api/crm.yaml Receipt.occurred_at":                              "when an approval was decided or a correction applied, not an activity",
	"api/crm.yaml WeeklyReviewDeal.occurred_at":                     "when a deal closed or moved stage, not an activity",
	"api/crm.yaml VoiceCorpusSource.occurred_at":                    "a voice sample's stated time, kept in voice_corpus_source and never drawn",
	"api/crm.yaml IngestVoiceCorpusSourceRequest.occurred_at":       "a voice sample's stated time, written to voice_corpus_source, not an activity",
	"api/crm.yaml AttentionItem.subject_moved_at": "a lane's own moment the item's subject last moved, compared only with when " +
		"the row was marked done; the description names occurred_at to say it is not that",
	"api/crm.yaml Company.last_activity_at":                          storedClock,
	"api/crm.yaml Contact.last_activity_at":                          storedClock,
	"api/crm.yaml Deal.last_activity_at":                             storedClock,
	"api/crm.yaml Project.last_activity_at":                          storedClock,
	"api/crm.yaml Project360Rollups.last_activity_at":                storedClock,
	"api/crm.yaml Company360Project.last_activity_at":                storedClock,
	"api/crm.yaml TeamWeeklyStaleDeal.last_activity_at":              storedClock,
	"api/crm.yaml ContactGraphEdge.last_at":                          storedClock,
	"api/crm.yaml ContactGraphRouteEvidence.last_at":                 storedClock,
	"api/crm.yaml ContactNetworkColleague.last_at":                   storedClock,
	"api/crm.yaml ContactNetworkColleague.last_inbound_at":           storedClock,
	"api/crm.yaml ContactNetworkColleague.last_outbound_at":          storedClock,
	"api/crm.yaml Company360Route.last_interaction_at":               storedClock,
	"api/crm.yaml Partner.last_contact_at":                           storedClock,
	"api/crm.yaml MorningBriefItemLineage.returned_with_activity_at": storedClock,
	"api/crm.yaml StageEvidence.observed_at":                         storedClock,
	"api/crm.yaml IntroRequest.replied_at": "copied from a connector-captured inbound mail when the ask was answered; " +
		"capture dates only an all-day calendar event, which has no direction",
	"api/crm.yaml DomainQuestion.evidence_at": "a stored clock from the captured mail that raised the question; " +
		"capture dates only an all-day calendar event, which is not mail",
	"api/crm.yaml ContactMoment.freshness_at":                 "the age key a moment is ordered by; no client draws it",
	"api/crm.yaml ContactMomentMayBeDone.wrote_at":            "the mail a promise may be kept by, as an ordering key; no client draws it",
	"api/crm.yaml SearchResult.works_at":                      "not a time: the company a contact works at",
	"api/crm.yaml UpdateRelationshipRequest.clear_ended_at":   "not a time: a flag that removes an end date",
	"api/crm.yaml UpdateRelationshipRequest.clear_started_at": "not a time: a flag that removes a start date",
	"api/public-events.yaml PublicEventEnvelope.occurred_at":  "when the event was emitted, the outbox's own instant",
})

// The reasons instantTimeNames gives, one per family of names.
const (
	rowStamp   = "a row's own write stamp on the server clock, not an activity's time"
	runStamp   = "when a pass, job, check or sync ran, the run's own instant, not an activity's time"
	aheadStamp = "a moment the server or a human sets ahead, a deadline or a schedule, not an activity's time"
	actStamp   = "when a human or the server acted on the row, that act's own instant, not an activity's time"
	spanStamp  = "where a run, project, employment or window starts or ends, not an activity's time"
)

// instantTimeNames are the names that are never an activity's time, wherever a
// schema declares them, each with the reason.
var instantTimeNames = gatekit.Waive(map[string]string{
	"created_at": rowStamp, "updated_at": rowStamp, "archived_at": rowStamp, "restricted_at": rowStamp,
	"content_erased_at": rowStamp, "superseded_at": rowStamp, "revoked_at": rowStamp, "suspended_at": rowStamp,
	"activated_at": rowStamp, "enabled_at": rowStamp, "added_at": rowStamp, "entered_at": rowStamp,
	"captured_at": rowStamp, "recorded_at": rowStamp, "set_at": rowStamp, "changed_at": rowStamp,
	"corrected_at": rowStamp, "effective_at": rowStamp, "latest_captured_at": rowStamp, "value_captured_at": rowStamp,

	"generated_at": runStamp, "computed_at": runStamp, "evaluated_at": runStamp, "checked_at": runStamp,
	"retrieved_at": runStamp, "estimated_at": runStamp, "assessed_at": runStamp, "narrated_at": runStamp,
	"annotated_at": runStamp, "finished_at": runStamp, "completed_at": runStamp, "failed_at": runStamp,
	"first_failed_at": runStamp, "dead_lettered_at": runStamp, "effect_failed_at": runStamp, "ran_at": runStamp,
	"detected_at": runStamp, "first_seen_at": runStamp, "observed_at": runStamp, "refresh_at": runStamp,
	"verified_at": runStamp, "connected_at": runStamp, "month_start_at": runStamp, "superseded_observed_at": runStamp,

	"expires_at": aheadStamp, "credential_expires_at": aheadStamp, "watch_expires_at": aheadStamp, "deletes_at": aheadStamp,
	"next_attempt_at": aheadStamp, "next_retry_at": aheadStamp, "next_pass_at": aheadStamp, "next_capture_at": aheadStamp,
	"next_sync_due_at": aheadStamp, "next_due_at": aheadStamp, "intended_due_at": aheadStamp, "resets_at": aheadStamp,
	"resumes_at": aheadStamp, "agent_window_next_starts_at": aheadStamp, "agent_window_resets_at": aheadStamp,
	"scheduled_at": aheadStamp, "remind_at": aheadStamp, "due_at": aheadStamp, "next_step_due_at": aheadStamp,
	"next_task_due_at": aheadStamp, "sla_deadline_at": aheadStamp,

	"accepted_at": actStamp, "approved_at": actStamp, "applied_at": actStamp, "decided_at": actStamp,
	"resolved_at": actStamp, "closed_at": actStamp, "submitted_at": actStamp, "requested_at": actStamp,
	"delivered_at": actStamp, "read_at": actStamp, "assigned_at": actStamp, "routed_at": actStamp,
	"promoted_at": actStamp, "qualified_at": actStamp, "taken_at": actStamp, "issued_at": actStamp,
	"paid_at": actStamp, "disposed_at": actStamp, "signed_in_at": actStamp, "visited_at": actStamp,
	"previous_visit_at": actStamp, "baseline_at": actStamp, "asked_at": actStamp, "introduced_at": actStamp,
	"name_dropped_at": actStamp, "first_response_at": actStamp, "responded_at": actStamp, "renewed_at": actStamp,
	"link_requested_at": actStamp, "double_opt_in_confirmed_at": actStamp, "state_at": actStamp,
	"refuted_at": actStamp, "left_at": actStamp, "received_at": actStamp, "acquired_at": actStamp, "done_at": actStamp,

	"started_at": spanStamp, "ended_at": spanStamp, "start_at": spanStamp, "end_at": spanStamp,
})

// wireTime is one `*_at` in a contract, and whether its precision sits beside it.
type wireTime struct {
	where, name, description string
	precise                  bool
}

func TestEveryActivityTimeOnTheWireCarriesItsPrecision(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	seen, precise := 0, 0
	for _, spec := range precisionContracts {
		raw, err := os.ReadFile(filepath.Join(root, spec)) // #nosec G304 -- a contract of this module
		if err != nil {
			t.Fatal(err)
		}
		for _, time := range wireTimesIn(t, spec, raw) {
			seen++
			if time.precise {
				precise++
				continue
			}
			judgeWireTime(t, time)
		}
	}
	precisionlessTimes.AssertAllMatched(t)
	instantTimeNames.AssertAllMatched(t)
	if seen < minWireTimes || precise == 0 {
		t.Errorf("the census found %d times, %d of them precise, want at least %d: it reads less of the contracts than it should",
			seen, precise, minWireTimes)
	}
}

// judgeWireTime places one time with no precision beside it, or fails.
func judgeWireTime(t *testing.T, time wireTime) {
	t.Helper()
	if precisionlessTimes.Waived(t, time.where) {
		return
	}
	if activityTimeNamed.MatchString(time.name) || activityTimeDescribed.MatchString(time.description) {
		t.Errorf("%s is an activity's time with no precision beside it, so a day entry reads as noon UTC. "+
			"Declare its `_precision` (OccurredPrecision) and fill it, or name in precisionlessTimes "+
			"why no day entry reaches it", time.where)
		return
	}
	if !instantTimeNames.Waived(t, time.name) {
		t.Errorf("%s is a time no list places. Declare its `_precision` if an activity's time reaches it, or say "+
			"why it is not one in instantTimeNames (by its name) or precisionlessTimes", time.where)
	}
}

// wireTimesIn reads every `*_at` one spec's schemas declare.
func wireTimesIn(t *testing.T, spec string, raw []byte) []wireTime {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", spec, err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatalf("%s declares no schemas, so this gate would read nothing", spec)
	}
	var out []wireTime
	for _, name := range slices.Sorted(maps.Keys(doc.Components.Schemas)) {
		out = append(out, schemaWireTimes(spec+" "+name, doc.Components.Schemas[name])...)
	}
	return out
}

// schemaWireTimes walks one schema and the objects nested in it.
//
//craft:ignore naked-any a YAML schema decodes to nested maps whose values are of any shape
func schemaWireTimes(path string, node any) []wireTime {
	object, isObject := node.(map[string]any)
	if !isObject {
		return nil
	}
	var out []wireTime
	if properties, ok := object["properties"].(map[string]any); ok {
		for _, name := range slices.Sorted(maps.Keys(properties)) {
			if strings.HasSuffix(name, "_at") {
				out = append(out, wireTime{
					where: path + "." + name, name: name,
					description: descriptionOf(properties[name]), precise: precisionBeside(name, properties),
				})
			}
			out = append(out, schemaWireTimes(path+"."+name, properties[name])...)
		}
	}
	for _, key := range []string{"items", "additionalProperties", "allOf", "oneOf", "anyOf"} {
		switch nested := object[key].(type) {
		case []any:
			for _, branch := range nested {
				out = append(out, schemaWireTimes(path, branch)...)
			}
		case map[string]any:
			out = append(out, schemaWireTimes(path, nested)...)
		}
	}
	return out
}

// precisionBeside says whether a sibling says how to read the time name: its
// own `<x>_precision`, or an `occurred_precision` for occurred_at or one whose
// description names it.
//
//craft:ignore naked-any the same decoded schema schemaWireTimes walks, for the same reason
func precisionBeside(name string, properties map[string]any) bool {
	if _, ok := properties[strings.TrimSuffix(name, "_at")+"_precision"]; ok {
		return true
	}
	occurred, ok := properties["occurred_precision"]
	return ok && (name == "occurred_at" || strings.Contains(descriptionOf(occurred), "`"+name+"`"))
}

//craft:ignore naked-any the same decoded schema schemaWireTimes walks, for the same reason
func descriptionOf(property any) string {
	described, _ := property.(map[string]any)
	description, _ := described["description"].(string)
	return description
}

// The census reads every time anywhere a schema nests it, the precision beside
// it, and which list must place it.
func TestThePrecisionCensusSeesEveryTime(t *testing.T) {
	t.Parallel()
	spec := []byte(`components:
  schemas:
    Precise:
      properties:
        occurred_at: {type: string}
        occurred_precision: {type: string}
    Bare:
      properties:
        occurred_at: {type: string}
    Nested:
      properties:
        rows:
          type: array
          items:
            properties:
              occurred_at: {type: string}
    Described:
      properties:
        last_touch_at: {type: string, description: "The activity's time of the last touch."}
    Named:
      properties:
        last_inbound_at: {type: string}
        last_inbound_precision: {type: string}
        meeting_moved_at: {type: string}
    Read:
      properties:
        starts_at: {type: string}
        occurred_precision: {type: string, description: "How to read ` + "`starts_at`" + `."}
    Other:
      properties:
        created_at: {type: string, description: "When the row was written."}
        sent_at: {type: string}
`)
	got := map[string]wireTime{}
	for _, time := range wireTimesIn(t, "seeded.yaml", spec) {
		got[time.where] = time
	}
	want := map[string]bool{
		"seeded.yaml Precise.occurred_at":     true,
		"seeded.yaml Bare.occurred_at":        false,
		"seeded.yaml Nested.rows.occurred_at": false,
		"seeded.yaml Described.last_touch_at": false,
		"seeded.yaml Named.last_inbound_at":   true,
		"seeded.yaml Named.meeting_moved_at":  false,
		"seeded.yaml Read.starts_at":          true,
		"seeded.yaml Other.created_at":        false,
		"seeded.yaml Other.sent_at":           false,
	}
	if len(got) != len(want) {
		t.Errorf("the census read %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
	for where, precise := range want {
		if found, ok := got[where]; !ok || found.precise != precise {
			t.Errorf("%s: read %+v (found %v), want precise %v", where, found, ok, precise)
		}
	}
	for where, activity := range map[string]bool{
		"seeded.yaml Bare.occurred_at": true, "seeded.yaml Described.last_touch_at": true,
		"seeded.yaml Named.meeting_moved_at": true, "seeded.yaml Other.created_at": false, "seeded.yaml Other.sent_at": false,
	} {
		time := got[where]
		if named := activityTimeNamed.MatchString(time.name) || activityTimeDescribed.MatchString(time.description); named != activity {
			t.Errorf("%s: read as an activity's time %v, want %v", where, named, activity)
		}
	}
}
