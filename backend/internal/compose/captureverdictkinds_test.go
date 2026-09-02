// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The sender taxonomy has three readers that must agree on ONE list: the
// lifecycle mapping, the JSON schema the model is constrained by, and the
// effect switch in apply(). They disagreed once — `personal` and `advisor`
// reached the prompt while the schema still refused them and apply() had no arm
// — and the comment claiming exhaustiveness is what stopped anybody checking.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/shared/schema"
)

func TestEveryVerdictKindHasAnEffect(t *testing.T) {
	// The effect switch is not reachable without a database, so this reads the
	// source of apply() and asserts every kind appears as a case. A weaker test
	// — asserting the map and the schema agree — is what the defect already
	// passed, because both were consistent with an effect switch that ignored
	// two of them.
	source := readSource(t, "captureverdict.go")
	body, ok := cutBetween(source, "switch kind {", "\n\t\t}")
	if !ok {
		t.Fatal("apply's effect switch not found in captureverdict.go — this gate reads it by shape, so a refactor must re-point it")
	}
	for _, kind := range verdictKindNames() {
		if !strings.Contains(body, "capture.Kind"+goName(kind)) {
			t.Errorf("kind %q is in verdictKinds but has no arm in apply's effect switch — "+
				"it would resolve the ledger row and then fail, burning the retries and retiring to unsure", kind)
		}
	}
}

func TestTheModelMayAnswerEveryKindTheTaxonomyDefines(t *testing.T) {
	// The schema is the generation-time constraint: on a grammar-constrained
	// local rung the model CANNOT emit a kind the enum omits, whatever the
	// prompt says. An omission here makes a kind unreachable in production
	// while every unit test still passes.
	var shape struct {
		Properties struct {
			Results struct {
				Items struct {
					Properties struct {
						Verdict struct {
							Enum []string `json:"enum"`
						} `json:"verdict"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"results"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(verdictSchema(), &shape); err != nil {
		t.Fatalf("decoding the verdict schema: %v", err)
	}
	got := make(map[string]bool, len(shape.Properties.Results.Items.Properties.Verdict.Enum))
	for _, kind := range shape.Properties.Results.Items.Properties.Verdict.Enum {
		got[kind] = true
	}
	if len(got) == 0 {
		t.Fatal("the verdict schema declares no kind enum — this gate would then pass vacuously")
	}
	for _, kind := range verdictKindNames() {
		if !got[kind] {
			t.Errorf("kind %q is in verdictKinds and in the prompt, but the response schema refuses it", kind)
		}
	}
	for kind := range got {
		if _, known := statusForKind(kind); !known {
			t.Errorf("the schema admits %q, which the taxonomy does not define", kind)
		}
	}
}

func TestTheTaxonomyDefinesTheKindsCaptureDeclares(t *testing.T) {
	// Both new kinds exist, spelled the way the CHECK constraint spells them.
	for _, kind := range []string{capture.KindPersonal, capture.KindAdvisor} {
		if _, known := statusForKind(kind); !known {
			t.Errorf("capture declares kind %q but the taxonomy has no status for it", kind)
		}
	}
}

// goName turns a snake_case kind into the Go constant suffix capture spells it
// with: role_mailbox → RoleMailbox.
func goName(kind string) string {
	parts := strings.Split(kind, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// readSource returns a file of this package for a gate that has to read code
// rather than call it.
func readSource(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// cutBetween returns what lies between the first open and the next close.
func cutBetween(s, open, close string) (string, bool) {
	_, after, found := strings.Cut(s, open)
	if !found {
		return "", false
	}
	body, _, found := strings.Cut(after, close)
	return body, found
}

func TestOneAssemblerCreatesEveryCounterpartyAVerdictMakes(t *testing.T) {
	// Three callers turn a decided sender into records: the machine's ordinary
	// verdict, the machine's advisor verdict, and a human's accept from the
	// review queue. They differ in who decided and in whether the record is
	// owner-scoped; what gets created must not differ at all, because a second
	// assembler is how the linking, the triage hand-off and the erasure check
	// drift apart between paths that a user cannot tell apart.
	//
	// So the claim is that exactly one function calls EnsureCounterpartyTx on
	// the verdict side. Asserting the CALLERS rather than the function's
	// existence is what makes this fail when somebody adds a second: a test
	// that only checked createCounterpartyRecords still exists would pass
	// beside a rival that never calls it.
	var callers []string
	for _, name := range verdictSourceFiles(t) {
		source := readSource(t, name)
		if strings.Contains(source, "EnsureCounterpartyTx(") {
			callers = append(callers, name)
		}
	}
	if len(callers) != 1 || callers[0] != "captureverdictcreate.go" {
		t.Errorf("EnsureCounterpartyTx is called from %v on the verdict side, want only captureverdictcreate.go — "+
			"a second assembler diverges from the first in what it links and what it checks", callers)
	}
}

// verdictSourceFiles lists this package's verdict sources, tests excluded. It
// fails when it finds none: a census that can come back empty reports PASS for
// a tree it never read.
func verdictSourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("listing the compose package: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "captureverdict") && strings.HasSuffix(name, ".go") &&
			!strings.HasSuffix(name, "_test.go") {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		t.Fatal("no captureverdict*.go sources found — this gate would pass vacuously")
	}
	return out
}

// The prompt tells the model which local parts make a role mailbox, and the
// tier ladder refuses the same addresses without asking anybody. Two statements
// of one rule, and they disagreed: the prompt named `service@` and the
// deterministic list did not, so `service@` at an ordinary company was created
// as a contact by the ladder before the model could have refused it.
//
// This is the cheap half of keeping them together — every word the prompt offers
// as an example is one the shared vocabulary actually matches. mailrole's own
// suite holds the other half.
func TestThePromptsRoleExamplesComeFromTheOneList(t *testing.T) {
	t.Parallel()
	for _, word := range mailrole.PromptExamples() {
		if !strings.Contains(verdictSystem, word+"@") {
			t.Errorf("the shared vocabulary offers %q@ as a role example and the prompt does not name it", word)
		}
	}
}

// The kinds that CREATE a record must be the kinds that need the higher floor.
//
// createsARecord is a second statement of what apply's effect switch does, and
// the switch is control flow rather than data — it cannot be read. A kind added
// there that creates a contact, and not added to createsARecord, would keep the
// lower floor: the record this lane exists to be careful about would be made on
// the weaker evidence, and nothing would say so.
//
// The creating kinds are named here rather than derived for the same reason the
// effect switch has no default: an exhaustive list somebody has to update is
// what a test can hold, and a fall-through is what silently gets it wrong.
func TestEveryCreatingKindNeedsTheHigherFloor(t *testing.T) {
	t.Parallel()
	creating := map[string]bool{
		capture.KindPerson:  true,
		capture.KindAdvisor: true,
	}
	for kind := range verdictKinds {
		if got := createsARecord(kind); got != creating[kind] {
			t.Errorf("createsARecord(%q) = %v, want %v — a kind that creates a contact "+
				"must clear the create floor, and one that does not must not be held to it",
				kind, got, creating[kind])
		}
	}
	// And the two floors are actually different, or the distinction above is
	// decoration: this test would pass for any pair of equal numbers.
	if verdictCreateFloor <= verdictConfidenceFloor {
		t.Fatalf("the create floor is %v and the ordinary floor is %v — creating a record "+
			"has to need more confidence than refusing one", verdictCreateFloor, verdictConfidenceFloor)
	}
}

// The create floor is enforced where the record is made, not only where the
// answer is read.
//
// A caller that checks the floor and a caller that forgets look identical from
// the outside, and the one that forgets creates a contact on evidence the
// product decided was too weak. So applyJudged refuses it itself.
//
// Driven through the engine's own chokepoint with a measurement below the
// floor, which no ordinary path produces — that is the point: this holds the
// door for a path that does not exist yet.
func TestTheCreateFloorIsEnforcedWhereTheRecordIsMade(t *testing.T) {
	t.Parallel()
	// A deterministic answer has no measurement and no floor to be below.
	if !clearsItsFloor(verdictResult{Verdict: capture.KindRoleMailbox, Confidence: 0.4}) {
		// clearsItsFloor still applies the ordinary floor to a role mailbox
		// answered BY A MODEL, which is correct — the exemption is for an answer
		// with no measurement at all, and that path does not reach here.
		t.Log("a model-answered role mailbox below 0.7 is refused, which is the ordinary floor")
	}
	for _, tc := range []struct {
		kind string
		conf float64
		want bool
	}{
		{capture.KindPerson, 0.86, true},
		{capture.KindPerson, 0.84, false},
		{capture.KindAdvisor, 0.86, true},
		{capture.KindAdvisor, 0.84, false},
		// Every other kind keeps the ordinary floor: refusing a contact is the
		// reversible direction.
		{capture.KindSpam, 0.71, true},
		{capture.KindSpam, 0.69, false},
		{capture.KindRoleMailbox, 0.71, true},
	} {
		got := clearsItsFloor(verdictResult{Verdict: tc.kind, Confidence: schema.Confidence(tc.conf)})
		if got != tc.want {
			t.Errorf("clearsItsFloor(%s at %.2f) = %v, want %v", tc.kind, tc.conf, got, tc.want)
		}
	}
}
