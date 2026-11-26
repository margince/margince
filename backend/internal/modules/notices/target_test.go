// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func TestOnlyACompleteStoredSignatureDescriptorSuppliesAMissingTarget(t *testing.T) {
	signature, owner := ids.NewV7(), ids.NewV7()
	valid := &systemtext.Message{Version: 1, Body: systemtext.Sentence("privacy.review.inactive", systemtext.Text(signature.String()), systemtext.Text(owner.String()))}
	named := Target{Type: "company", ID: ids.NewV7()}
	for _, tc := range []struct {
		name, kind string
		stored     Target
		message    *systemtext.Message
		want       Target
	}{
		{"legacy descriptor", KindPrivacyReviewRequired, Target{}, valid, Target{Type: TargetRetainedEmailSignature, ID: signature}},
		{"explicit reference", KindPrivacyReviewRequired, named, valid, named},
		{"different notice kind", "automation", Target{}, valid, Target{}},
		{"no descriptor", KindPrivacyReviewRequired, Target{}, nil, Target{}},
		{"unknown descriptor version", KindPrivacyReviewRequired, Target{}, &systemtext.Message{Version: 2, Body: valid.Body}, Target{}},
		{"invalid signature reference", KindPrivacyReviewRequired, Target{}, &systemtext.Message{Version: 1, Body: systemtext.Sentence("privacy.review.inactive", systemtext.Text("invalid"), systemtext.Text(owner.String()))}, Target{}},
		{"missing owner reference", KindPrivacyReviewRequired, Target{}, &systemtext.Message{Version: 1, Body: systemtext.Sentence("privacy.review.inactive", systemtext.Text(signature.String()))}, Target{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvedNoticeTarget(tc.stored, tc.kind, tc.message); got != tc.want {
				t.Fatalf("reference=%+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOwnTargetsAreTheDeclaredConstants(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "target.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "OwnTargetType" {
			return true
		}
		for _, v := range spec.Values {
			value, err := strconv.Unquote(v.(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			declared[value] = true
		}
		return true
	})
	if len(declared) != len(ownTargets) {
		t.Errorf("OwnTargetType declares %d values and ownTargets holds %d", len(declared), len(ownTargets))
	}
	for value := range declared {
		if _, ok := ownTargets[value]; !ok {
			t.Errorf("OwnTargetType declares %q and ownTargets does not", value)
		}
	}
}

func TestANoticeRefusesATargetTypeTheCheckWouldRefuse(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	for _, accepted := range []Target{{}, {Type: "deal", ID: id}, {Type: TargetApproval, ID: id}, {Type: "approval_history", ID: id}, {Type: "ext_visit_log_visit", ID: id}} {
		if err := requireKnownTarget(accepted); err != nil {
			t.Errorf("%+v was refused: %v", accepted, err)
		}
	}
	for _, refused := range []string{"widget", "Deal", "ext:visit-log:visit"} {
		if err := requireKnownTarget(Target{Type: refused, ID: id}); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%q: want ErrInvalidArgument, got %v", refused, err)
		}
	}
}
