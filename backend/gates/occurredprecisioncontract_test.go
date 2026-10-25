// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every activity time on the wire says how precisely it is known.
//
// A day entry is stored at noon UTC of its date. A projection that hands out
// that instant with no precision beside it reads as "12:00" to every client.
// The frontend's precision gate cannot see a field the contract never declared.
// So a schema's `occurred_at`, or a `<x>_at` its description calls an
// activity's time, has `occurred_precision` or `<x>_precision` beside it.

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

// activityTimeDescribed is a description that calls a `<x>_at` an activity's
// time, or names the `occurred_at` it is taken from.
var activityTimeDescribed = regexp.MustCompile("(?i)\\b(?:activity|activities)'?s? time\\b|`occurred_at`")

// minActivityTimes is the floor: fewer activity times than this means the walk
// reads less of the contract than it should.
const minActivityTimes = 40

// precisionlessActivityTimes are the times the census reads as an activity's
// that carry no precision, each with the reason. Keyed "<spec> <schema
// path>.<property>" as the census prints it.
var precisionlessActivityTimes = gatekit.Waive(map[string]string{
	"api/crm.yaml AiCallAttempt.occurred_at":     "when a model call was made, an ai_call row's own instant",
	"api/crm.yaml AiCallSummary.occurred_at":     "when a model call was made, an ai_call row's own instant",
	"api/crm.yaml AuditHistoryEntry.occurred_at": "when an audit row was written, the write's own instant",
	"api/crm.yaml AuditLogEntry.occurred_at":     "when an audit row was written, the write's own instant",
	"api/crm.yaml AutomationRun.occurred_at":     "when an automation ran, the run's own instant",
	"api/crm.yaml CaptureTraceEntry.occurred_at": "when capture logged a step, the trace row's own instant",
	"api/crm.yaml ConsentEvent.occurred_at":      "when consent changed, the ledger row's own instant",
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
	"api/crm.yaml ContactGraphEdge.last_at": "a sort key the relationship map orders edges by and never draws; the edge " +
		"tables fold it from many activities at write time",
	"api/crm.yaml Company.last_activity_at":                  "a stored clock with no precision column; whether it gains one is #8910",
	"api/crm.yaml Contact.last_activity_at":                  "a stored clock with no precision column; whether it gains one is #8910",
	"api/crm.yaml Deal.last_activity_at":                     "a stored clock with no precision column; whether it gains one is #8910",
	"api/public-events.yaml PublicEventEnvelope.occurred_at": "when the event was emitted, the outbox's own instant",
})

// activityTime is one activity time in a contract, and whether its precision
// sits beside it.
type activityTime struct {
	where   string
	precise bool
}

func TestEveryActivityTimeOnTheWireCarriesItsPrecision(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	seen := 0
	for _, spec := range precisionContracts {
		raw, err := os.ReadFile(filepath.Join(root, spec)) // #nosec G304 -- a contract of this module
		if err != nil {
			t.Fatal(err)
		}
		for _, time := range activityTimesIn(t, spec, raw) {
			seen++
			if !time.precise && !precisionlessActivityTimes.Waived(t, time.where) {
				t.Errorf("%s is an activity's time with no precision beside it, so a day entry reads as noon UTC. "+
					"Declare its `_precision` (OccurredPrecision) and fill it, or name in precisionlessActivityTimes "+
					"why no day entry reaches it", time.where)
			}
		}
	}
	precisionlessActivityTimes.AssertAllMatched(t)
	if seen < minActivityTimes {
		t.Errorf("the census found %d activity times, want at least %d: it reads less of the contracts than it should",
			seen, minActivityTimes)
	}
}

// activityTimesIn reads every activity time one spec's schemas declare.
func activityTimesIn(t *testing.T, spec string, raw []byte) []activityTime {
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
	var out []activityTime
	for _, name := range slices.Sorted(maps.Keys(doc.Components.Schemas)) {
		out = append(out, schemaActivityTimes(spec+" "+name, doc.Components.Schemas[name])...)
	}
	return out
}

// schemaActivityTimes walks one schema and the objects nested in it.
//
//craft:ignore naked-any a YAML schema decodes to nested maps whose values are of any shape
func schemaActivityTimes(path string, node any) []activityTime {
	object, isObject := node.(map[string]any)
	if !isObject {
		return nil
	}
	var out []activityTime
	if properties, ok := object["properties"].(map[string]any); ok {
		for _, name := range slices.Sorted(maps.Keys(properties)) {
			if precision, isTime := precisionOf(name, properties[name]); isTime {
				_, precise := properties[precision]
				out = append(out, activityTime{where: path + "." + name, precise: precise})
			}
			out = append(out, schemaActivityTimes(path+"."+name, properties[name])...)
		}
	}
	for _, key := range []string{"items", "additionalProperties", "allOf", "oneOf", "anyOf"} {
		switch nested := object[key].(type) {
		case []any:
			for _, branch := range nested {
				out = append(out, schemaActivityTimes(path, branch)...)
			}
		case map[string]any:
			out = append(out, schemaActivityTimes(path, nested)...)
		}
	}
	return out
}

// precisionOf names the precision property an activity time needs beside it,
// and says whether name is one.
//
//craft:ignore naked-any the same decoded schema schemaActivityTimes walks, for the same reason
func precisionOf(name string, property any) (string, bool) {
	if name == "occurred_at" {
		return "occurred_precision", true
	}
	stem, isTime := strings.CutSuffix(name, "_at")
	if !isTime {
		return "", false
	}
	described, _ := property.(map[string]any)
	description, _ := described["description"].(string)
	return stem + "_precision", activityTimeDescribed.MatchString(description)
}

// The census finds an activity time anywhere a schema nests it, and reads its
// precision beside it.
func TestThePrecisionCensusSeesEveryActivityTime(t *testing.T) {
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
    Other:
      properties:
        created_at: {type: string, description: "When the row was written."}
`)
	got := map[string]bool{}
	for _, time := range activityTimesIn(t, "seeded.yaml", spec) {
		got[time.where] = time.precise
	}
	want := map[string]bool{
		"seeded.yaml Precise.occurred_at":     true,
		"seeded.yaml Bare.occurred_at":        false,
		"seeded.yaml Nested.rows.occurred_at": false,
		"seeded.yaml Described.last_touch_at": false,
	}
	if len(got) != len(want) {
		t.Errorf("the census read %v, want %v", got, want)
	}
	for where, precise := range want {
		if found, ok := got[where]; !ok || found != precise {
			t.Errorf("%s: read %v (found %v), want precise %v", where, found, ok, precise)
		}
	}
}
