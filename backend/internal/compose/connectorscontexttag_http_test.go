// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The wire half of the context-tag surface: what a caller gets back when the
// thing they addressed is not theirs, and when nothing is wired at all.
//
// Both are answers a client acts on rather than internal detail. A mailbox this
// caller has not connected must read as ABSENT and not as a refusal — a 403
// would tell them a connection exists that they may not touch, which is a fact
// about somebody else's mailbox.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asSeat binds a human, because the registry refuses everyone else before it
// looks at a connection at all — so a request with no actor would be answered
// by the wrong gate and prove nothing about this handler.
func asSeat(req *http.Request) *http.Request {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:seat", UserID: ids.NewV7(),
	})
	return req.WithContext(ctx)
}

// A registry with no connector registered under the requested name answers
// ErrNoConnection, which is the shape this handler has to map.
func TestSettingAWordOnAMailboxTheCallerHasNotConnectedReadsAsAbsent(t *testing.T) {
	h := connectorHandlers{registry: capture.NewRegistry(nil, nil, deadAuthority{}, nil)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/connectors/gmail/context-tag",
		strings.NewReader(`{"tag_id":null}`))
	req.Header.Set("Content-Type", "application/json")
	h.SetConnectorContextTag(rec, asSeat(req), "gmail")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 — a mailbox this caller has not connected is not theirs to "+
			"configure, and a 403 would confirm that somebody else's connection exists", rec.Code)
	}
}

// An installation with no capture registry composed answers 501, the same as
// every other seam this build did not wire. Not 500: nothing failed.
func TestSettingAWordWithNoRegistryComposedIsUnwiredRatherThanBroken(t *testing.T) {
	var h connectorHandlers

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/connectors/gmail/context-tag",
		strings.NewReader(`{"tag_id":null}`))
	req.Header.Set("Content-Type", "application/json")
	h.SetConnectorContextTag(rec, req, "gmail")

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501 — an unwired seam is not a failure", rec.Code)
	}
}

// A body the contract does not accept is refused before the registry is asked,
// so a malformed request can never reach a write.
func TestAMalformedBodyIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	h := connectorHandlers{registry: capture.NewRegistry(nil, nil, deadAuthority{}, nil)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/connectors/gmail/context-tag",
		strings.NewReader(`{"tag_id":`))
	req.Header.Set("Content-Type", "application/json")
	h.SetConnectorContextTag(rec, req, "gmail")

	if rec.Code == http.StatusOK {
		t.Error("a malformed body answered 200")
	}
	if rec.Code >= http.StatusInternalServerError {
		t.Errorf("status = %d — a body the caller got wrong is not the server failing", rec.Code)
	}
}
