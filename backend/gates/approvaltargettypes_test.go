// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// An approval's target type is the closed list approvals.TargetType declares, and
// enumsync_test.go holds the CHECK on approval.target_entity_type equal to it.
// Neither says whether a writer stages a type the list leaves out; that writer
// would fail its insert. This gate holds the entity vocabulary and the agent
// policy's record types inside the list, and names the discriminator columns
// that stay free text.

import (
	"bufio"
	"bytes"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const approvalTargetTypes = "internal/modules/approvals.TargetType"

func TestEveryApprovalWriterVocabularyIsAnApprovalTargetType(t *testing.T) {
	t.Parallel()
	_, typed := vocabularyCensusInputs(t)
	declared := typed[approvalTargetTypes]
	if len(declared) == 0 {
		t.Fatalf("no %s constants found: the derivation broke", approvalTargetTypes)
	}
	writers := map[string]string{
		"internal/shared/ports/datasource.EntityType": "the entity vocabulary every staged record edit names",
		"internal/compose.agentRecordType":            "the record types an agent tool stages against",
	}
	for source, why := range writers {
		values := typed[source]
		if len(values) == 0 {
			t.Errorf("no %s constants found: the derivation broke, and the gate would read green over nothing", source)
		}
		for _, value := range values {
			if !slices.Contains(declared, value) {
				t.Errorf("%s holds %q (%s), which approvals.TargetType does not. Staging it would fail "+
					"the approval CHECK. Add it to TargetType and to the CHECK together", source, value, why)
			}
		}
	}
}

// freeTextDiscriminators are the polymorphic type columns that keep no CHECK.
// Each is a log of what a thing was about when it happened. A closed list would
// refuse the row on the day a new kind of thing appears.
var freeTextDiscriminators = gatekit.Waive(map[string]string{
	"audit_log.entity_type":        "the audit log records every entity type the product has ever audited, including ones since removed, and a refused insert would drop the audit row",
	"ai_call.subject_type":         "the AI call log records what a model call was about; the row is kept after the subject and its kind are gone",
	"ai_task_run.subject_type":     "the AI run log records what a run was about; the row is kept after the subject and its kind are gone",
	"webhook_delivery.entity_type": "the webhook log records what a delivery was about, for every event type a subscriber was ever sent",
	"webhook_delivery.event_type":  "the webhook log records the event name as sent; a new event type must not make its delivery row fail",
})

var catalogTextColumnLine = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*) text\b`)

func TestAFreeTextDiscriminatorHasNoVocabularyCheck(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	textColumns := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if m := catalogTextColumnLine.FindStringSubmatch(scanner.Text()); m != nil {
			textColumns[m[1]] = true
		}
	}
	vocabularies, err := catalogVocabularies()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range freeTextDiscriminators.Subjects() {
		freeTextDiscriminators.Waived(t, column)
		if !textColumns[column] {
			t.Errorf("%s is not a text column in %s: remove its entry", column, vocabularyCatalog)
		}
		if _, hasCheck := vocabularies[column]; hasCheck {
			t.Errorf("%s now carries a vocabulary CHECK: remove its entry, and bind it in enumBindings", column)
		}
	}
	freeTextDiscriminators.AssertAllMatched(t)
}
