// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// What a seat may decide about being interrupted, decided before any database
// is involved: which class a produced kind lands in, what the installation
// decides for somebody who never chose, and the choices the vocabulary refuses.
//
// The store here holds no database, so a refusal that did not precede the query
// would panic rather than pass — the same proof store_test.go leans on.

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestClassForPlacesEveryKindAProducerRaises(t *testing.T) {
	t.Parallel()
	// The kinds as their PRODUCERS spell them, not as the map does: a module
	// may not import compose, so this table is the only place the two meet.
	for _, tc := range []struct {
		kind  string
		class string
	}{
		{"automation", classAutomation},
		{"lead_sla", classLeadSLA},
		{"capture_backlog_stalled", classCapture},
		{"vcard_staging_failed", classSystem},
		{KindApprovalPending, ClassApprovalPending},
		{string(crmcontracts.NoticeKindCoachReplyAging), classCoach},
		{string(crmcontracts.NoticeKindCoachDealNeedsNextStep), classCoach},
		{string(crmcontracts.NoticeKindCoachReviewBacklog), classCoach},
		{string(crmcontracts.NoticeKindCoachGeneral), classCoach},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			class, err := ClassFor(tc.kind)
			if err != nil {
				t.Fatalf("ClassFor(%q): %v — a kind a producer raises has no class, so its notice is delivered under a setting nobody chose", tc.kind, err)
			}
			if class != tc.class {
				t.Fatalf("ClassFor(%q) = %q, want %q", tc.kind, class, tc.class)
			}
		})
	}
}

func TestClassForRefusesAKindNothingRaises(t *testing.T) {
	t.Parallel()
	if class, err := ClassFor("gossip"); err == nil {
		t.Fatalf("ClassFor(\"gossip\") = %q with no error — an unplaced kind would be delivered as though somebody had decided about it", class)
	}
}

// The enumeration that picks the morning's seats runs before any of them is
// claimed, so a kind it cannot place must cost that one line and not the
// installation's whole morning — a notice written under a kind since retired
// from the contract is a row nobody can edit.
//
// NO DATABASE, and that is the assertion: the refusal has to precede the
// preference query, so the nil transaction is what proves it rather than a
// shortcut. An edit that looked the class up first would panic here.
func TestARetiredKindIsNotBatchedRatherThanFatal(t *testing.T) {
	t.Parallel()
	if _, err := ClassFor("gossip"); err == nil {
		t.Fatal("\"gossip\" is placeable now, so this case no longer stands for a kind the product cannot place")
	}

	batched := newBatchedClasses(ids.New[ids.UserKind]())
	wanted, err := batched.wants(context.Background(), nil, "gossip")
	if err != nil {
		t.Fatalf("asking whether a retired kind batches: %v — one such row would cost every seat its digest", err)
	}
	if wanted {
		t.Fatal("a kind with no class batched — it would reach a reader under a setting nobody chose")
	}
}

// A coach kind the contract admits and this package does not place would reach
// a reader under whatever the fallback happened to be. The kinds are read off
// the generated enum rather than typed out, so a fifth one arrives in this test
// without anybody remembering to add it.
func TestEveryCoachKindTheContractPublishesIsClassified(t *testing.T) {
	t.Parallel()
	kinds := publishedCoachKinds(t)
	// The floor is derived, not guessed: this package already answers for one
	// subject per coach kind, so a scan finding fewer has read a smaller tree
	// than the one it is judging — the census that fails short and reports PASS.
	if len(kinds) < len(coachSubjects) {
		t.Fatalf("the contract scan found %d coaching kinds and this package answers for %d — the scan is reading less than the tree holds", len(kinds), len(coachSubjects))
	}
	for _, kind := range kinds {
		class, err := ClassFor(kind)
		if err != nil {
			t.Fatalf("ClassFor(%q): %v — the contract publishes this coaching kind", kind, err)
		}
		if class != classCoach {
			t.Fatalf("ClassFor(%q) = %q, want the coaching class", kind, class)
		}
	}
}

// publishedCoachKinds reads the contract's NoticeKind values off the GENERATED
// constants, the way the activity-kind census does: crm.yaml declares the
// vocabulary and the drift gate already fails when the generated type stops
// matching it, so reading the constants is reading the contract one hop away.
func publishedCoachKinds(t *testing.T) []string {
	t.Helper()
	const generated = "../../contracts/api_gen.go"
	file, err := parser.ParseFile(token.NewFileSet(), generated, nil, 0)
	if err != nil {
		t.Fatalf("reading the generated contract: %v", err)
	}
	var kinds []string
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Values) != 1 {
				continue
			}
			named, isNamed := value.Type.(*ast.Ident)
			if !isNamed || named.Name != "NoticeKind" {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				continue
			}
			kind, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("reading the value of %s: %v", named.Name, err)
			}
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// A class a kind can arrive as but nobody can decide about is a setting page
// with a hole in it: the notice lands, and the reader has no row to route it
// with.
func TestEveryClassAKindLandsInCanBeDecidedAbout(t *testing.T) {
	t.Parallel()
	for kind, class := range classByKind {
		if !slices.Contains(noticeClasses, class) {
			t.Errorf("kind %q lands in class %q, which the settings set does not offer", kind, class)
		}
	}
	if !slices.Contains(noticeClasses, classCoach) {
		t.Error("the coaching class is not offered, so a reader cannot route a colleague's words at all")
	}
}

// The save decides whether anything moved, and what the ledger says it moved
// from, out of a read — and then writes what it decided. Both halves must see
// ONE state, and the write identity is what makes that true rather than
// intended: without it two tabs saving the same class each read "nothing
// decided here", each pass the no-op skip, and the second records a change from
// a null the first had already filled.
//
// Read from the SOURCE rather than raced for. What a running database would
// prove is that an advisory lock blocks, which is Postgres's contract and not
// ours; what can regress here is taking the lock after the read, or not taking
// it — and that is a property of the text. Capture's backfill lock-order test
// makes the same call for the same reason.
func TestTheSaveTakesTheSeatsWriteIdentityBeforeItReadsOrWrites(t *testing.T) {
	t.Parallel()
	const guard = "LockWriteIdentity"
	dependents := []string{"chosenBy", "savePreference"}
	calls := callsInOrder(t, "preference.go", "SaveNotificationPreference")
	// Nothing here may pass short: a rename this test stopped recognising would
	// otherwise leave it asserting an order between names that are not there.
	for _, name := range append([]string{guard}, dependents...) {
		if !slices.Contains(calls, name) {
			t.Fatalf("the save calls no %s — this test is reading a function it no longer describes: %v", name, calls)
		}
	}
	guardAt := slices.Index(calls, guard)
	for _, name := range dependents {
		if slices.Index(calls, name) < guardAt {
			t.Errorf("%s runs before %s, so the save decides from a read a concurrent save can "+
				"invalidate between the two statements", name, guard)
		}
	}
}

// callsInOrder lists the functions the named function calls, in source order,
// spelled by the name at the call site — a selector reads as its final name, so
// storekit.LockWriteIdentity and a bare helper are both just their own name.
func callsInOrder(t *testing.T, file, fn string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	var calls []string
	for _, decl := range parsed.Decls {
		declared, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || declared.Name.Name != fn {
			continue
		}
		ast.Inspect(declared.Body, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			switch called := call.Fun.(type) {
			case *ast.Ident:
				calls = append(calls, called.Name)
			case *ast.SelectorExpr:
				calls = append(calls, called.Sel.Name)
			}
			return true
		})
	}
	if len(calls) == 0 {
		t.Fatalf("%s declares no %s, or it calls nothing", file, fn)
	}
	return calls
}

func TestOnlyAnApprovalLeavesTheProductByDefault(t *testing.T) {
	t.Parallel()
	if got := DefaultDelivery(ClassApprovalPending); got != DeliveryEmail {
		t.Fatalf("an unchosen approval defaults to %q — the one notice that blocks a colleague has to leave the screen", got)
	}
	for _, class := range noticeClasses {
		if class == ClassApprovalPending {
			continue
		}
		if got := DefaultDelivery(class); got != DeliveryInApp {
			t.Errorf("an unchosen %q defaults to %q, want the screen the reader is already looking at", class, got)
		}
	}
}

func TestASaveRefusesAChoiceTheVocabularyDoesNotHold(t *testing.T) {
	t.Parallel()
	store := NewStore(nil)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:x", UserID: ids.NewV7(),
	})
	// The CODE as well as the field: two of these refusals name the same field,
	// and a caller shown "delivery" alone cannot tell a word outside the
	// vocabulary from a word this class in particular will not take.
	for _, tc := range []struct {
		name     string
		class    string
		delivery string
		field    string
		code     string
	}{
		{"a class nothing produces", "gossip", DeliveryInApp, "class", "unknown"},
		{"a transport that does not exist", classAutomation, "carrier_pigeon", "delivery", "unknown"},
		{"muting a colleague's coaching", classCoach, DeliveryOff, "delivery", "value_not_allowed"},
		{"mailing a class with no sending leg", classAutomation, DeliveryEmail, "delivery", "value_not_allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := store.SaveNotificationPreference(ctx, tc.class, tc.delivery)
			var parse *values.ParseError
			if !errors.As(err, &parse) {
				t.Fatalf("saving (%q, %q) = %v, want a refusal naming the field", tc.class, tc.delivery, err)
			}
			if parse.Field != tc.field {
				t.Fatalf("the refusal names %q, want %q — a caller matching on the name cannot tell a typo from another field", parse.Field, tc.field)
			}
			if parse.Code != tc.code {
				t.Fatalf("the refusal carries code %q, want %q", parse.Code, tc.code)
			}
		})
	}
}

// IMMEDIATE MAIL IS THE APPROVAL CLASS'S ALONE, because it is the only class a
// producer stages a message for. Taking the word anywhere else would record a
// choice the product does not keep: the reader would stop watching the screen
// and wait for mail nothing was ever going to send.
//
// The refusal narrows the ROUTE and not the possibility, which the second half
// holds: every class still reaches a mailbox through the daily batch.
func TestOnlyTheApprovalClassTakesImmediateMail(t *testing.T) {
	t.Parallel()
	for _, class := range noticeClasses {
		err := checkPreference(class, DeliveryEmail)
		if class == ClassApprovalPending {
			if err != nil {
				t.Errorf("the one class with a sending leg refuses email: %v", err)
			}
			continue
		}
		var parse *values.ParseError
		if !errors.As(err, &parse) || parse.Code != "value_not_allowed" {
			t.Errorf("%q accepts email and nothing mails it, so the reader waits for a message that never comes: %v", class, err)
		}
	}
	for _, class := range noticeClasses {
		if err := checkPreference(class, DeliveryDigest); err != nil {
			t.Errorf("%q cannot be batched, so it reaches a mailbox by no route at all: %v", class, err)
		}
	}
}

// THE MUTING INVERSION HAS A BLIND SPOT, AND THIS IS WHAT KEEPS IT HARMLESS.
//
// mutedKinds reads classByKind, which places every kind a system flow raises.
// The coaching kinds are absent from it — ClassFor places those off the
// contract's own vocabulary — so no coaching kind can ever be named as muted.
// That is only safe while coach is a class nobody may switch off.
//
// The day it stopped being true nothing else would fail: the lane would quietly
// go on carrying a class its reader had switched off, and no assertion anywhere
// would be looking. Derived from the two maps rather than naming coach, so a
// sixth class placed the same way is covered the day it appears.
func TestAClassOutsideTheKindMapCannotBeMuted(t *testing.T) {
	t.Parallel()
	placed := map[string]bool{}
	for _, class := range classByKind {
		placed[class] = true
	}
	for _, class := range noticeClasses {
		if placed[class] {
			continue
		}
		if err := checkPreference(class, DeliveryOff); err == nil {
			t.Errorf("%q may be switched off and no kind places into it, so mutedKinds can never "+
				"name it and the reader's lane would go on carrying the class they muted", class)
		}
	}
}
