// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"testing"
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
