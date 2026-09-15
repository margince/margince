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
