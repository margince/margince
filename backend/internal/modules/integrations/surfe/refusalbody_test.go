// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package surfe

// A vendor refusal does not promise JSON. A gateway in front of Surfe answers
// an expired key with its own HTML. Read as a decode fault, that tells the
// customer their provider changed shape rather than that their key was refused.
//
// It also reaches the callers that read an error as an outcome. A submission
// parks as ambiguous with its reservation held, and a poll reads as unfinished
// until the run expires.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// gatewayHTML is what a refusal looks like when it never reached the vendor's
// own application: a proxy's error page, with no documented field in it.
const gatewayHTML = `<html><head><title>401 Authorization Required</title></head><body></body></html>`

func TestARefusalWithANonJSONBodyNamesTheRefusal(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		rec := &recorder{status: status, body: gatewayHTML}
		_, err := testAdapter(rec).VerifyCredential(context.Background(), provider.Credential("expired"))
		if !errors.Is(err, ErrCredentialRefused) {
			t.Errorf("a %d carrying HTML gave %v, want ErrCredentialRefused: the status is the answer", status, err)
		}
	}
}

// The decode still runs where the vendor claimed success. A 200 that is not the
// documented shape is a shape fault, and the caller has nothing to read.
func TestASuccessWithANonJSONBodyStillReadsAsAShapeFault(t *testing.T) {
	t.Parallel()
	rec := &recorder{status: http.StatusOK, body: gatewayHTML}
	_, err := testAdapter(rec).VerifyCredential(context.Background(), provider.Credential("k"))
	if err == nil {
		t.Fatal("a 200 with an undecodable body verified successfully")
	}
	if errors.Is(err, ErrCredentialRefused) {
		t.Errorf("err = %v, want the shape fault rather than a refusal: the vendor accepted the key", err)
	}
}

// A refused submission is a decided outcome. Read as an error it became
// ambiguous, which parks the run with its reservation held. The hold stands
// against a key the vendor had already rejected.
func TestARefusedSubmissionWithANonJSONBodyIsDecidedRatherThanAmbiguous(t *testing.T) {
	t.Parallel()
	rec := &recorder{status: http.StatusUnauthorized, body: gatewayHTML}
	got, err := testAdapter(rec).Submit(context.Background(), provider.Credential("expired"), aRequest())
	if err != nil {
		t.Fatalf("Submit returned %v, want the refusal as an outcome", err)
	}
	if got.Outcome == provider.OutcomeAmbiguous {
		t.Errorf("outcome = %q with code %q, want a decided refusal", got.Outcome, got.SafeStatusCode)
	}
}

// A refused poll is also decided. Read as an error it reported pending, so the
// sweep asked again every tick until the run expired. Each tick presented the
// key the vendor had refused.
func TestARefusedPollWithANonJSONBodyDoesNotReadAsUnfinished(t *testing.T) {
	t.Parallel()
	rec := &recorder{status: http.StatusUnauthorized, body: gatewayHTML}
	got, err := testAdapter(rec).Poll(context.Background(), provider.Credential("expired"), "job-1")
	if err != nil {
		t.Fatalf("Poll returned %v, want the refusal as an outcome", err)
	}
	if got.Outcome == provider.OutcomePending {
		t.Error("a refused poll read as pending, so the sweep would re-present a refused key until expiry")
	}
}
