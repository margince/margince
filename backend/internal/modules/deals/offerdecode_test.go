// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// An offer body that does not decode is refused in the caller's words, as
// every other create is. It names the field and shape, never the Go types.
func TestAnUndecodableOfferBodyIsRefusedInTheCallersWords(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]string{
		`{"currency":5,"source":"manual"}`: "currency",
		`[]`:                               "not an array",
		`{"currency":"EUR","source":"manual","line_items":[1]}`: "line_items",
		``: "empty",
	} {
		_, err := OfferCreationInput(t.Context(), []byte(body))
		var refusal *httperr.DetailedError
		if !errors.As(err, &refusal) {
			t.Fatalf("%s answered %v, want a validation refusal", body, err)
		}
		detail := refusal.Error()
		for _, internal := range []string{"Go ", "crmcontracts", "CreateOfferRequest", "unmarshal"} {
			if strings.Contains(detail, internal) {
				t.Errorf("%s: the refusal shows %q: %s", body, internal, detail)
			}
		}
		if !strings.Contains(detail, want) {
			t.Errorf("%s: the refusal %q does not name %q", body, detail, want)
		}
	}
}
