// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The passport door of a case merge decodes the contract body, so a member
// named twice is the same 422 the session door answers.
func TestACaseMergeOnThePassportDoorRefusesAMemberNamedTwice(t *testing.T) {
	contact := ids.NewV7()
	body := []byte(strings.Replace(contactCaseBody,
		"019ff000-0000-7000-8000-000000000052", "019ff000-0000-7000-8000-000000000051", 1))
	req := httptest.NewRequest(http.MethodPost, "/v1/dedupe/contact-cases/"+contact.String()+"/merge", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contact_id", contact.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	for _, op := range []string{"mergeContactCase", "previewContactCaseMerge"} {
		_, err := restCommands[op](agentPolicy{}, restCommandDeps{records: seamRecord{}}, req, body)
		var refusal *httperr.DetailedError
		if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity ||
			len(refusal.Fields) != 1 || refusal.Fields[0].Field != "member_ids" {
			t.Errorf("%s with a repeated member answered %v, want a 422 naming member_ids", op, err)
		}
	}
}
