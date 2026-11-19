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

// The passport door answers a repeated member and an agent's preference with a 422.
func TestACaseMergeOnThePassportDoorRefusesABodyItCannotTake(t *testing.T) {
	contact := ids.NewV7()
	req := httptest.NewRequest(http.MethodPost, "/v1/dedupe/contact-cases/"+contact.String()+"/merge", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contact_id", contact.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	for field, body := range map[string]string{
		"member_ids": strings.Replace(contactCaseBody, "019ff000-0000-7000-8000-000000000052", "019ff000-0000-7000-8000-000000000051", 1),
		"prefer":     strings.Replace(contactCaseBody, `"owner_id"`, `"prefer":{"email":"019ff000-0000-7000-8000-000000000052"},"owner_id"`, 1),
	} {
		for _, op := range []string{"mergeContactCase", "previewContactCaseMerge"} {
			_, err := restCommands[op](agentPolicy{}, restCommandDeps{records: seamRecord{}}, req, []byte(body))
			var refusal *httperr.DetailedError
			if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity || len(refusal.Fields) != 1 || refusal.Fields[0].Field != field {
				t.Errorf("%s answered %v, want a 422 naming %s", op, err, field)
			}
		}
	}
}
