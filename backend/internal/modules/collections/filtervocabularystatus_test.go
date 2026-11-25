// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

func TestAKindNoExtensionDeclaresAnswers404OnTheVocabularyAndATypoStays422(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"list": {Read: true}}},
	})
	for resource, want := range map[string]int{
		"ext:alpha:visit": http.StatusNotFound,
		"peron":           http.StatusUnprocessableEntity,
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/v1/filters/vocabulary", nil).WithContext(reader)
		NewHandlers(&Store{}).GetFilterVocabulary(w, r, crmcontracts.GetFilterVocabularyParams{Resource: crmcontracts.FilterResource(resource)})
		if w.Code != want {
			t.Errorf("resource %q answered %d, want %d", resource, w.Code, want)
		}
	}
}
