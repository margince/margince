// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type filterProbeInput struct{ Owner *ids.UserID }

// A list filter of the wrong shape is the caller's mistake on every surface.
func TestAMalformedListFilterIsAValidationRefusal(t *testing.T) {
	set := storekit.FilterSet[filterProbeInput]{
		"owner_id": storekit.FilterID(func(in *filterProbeInput, id *ids.UserID) { in.Owner = id }),
	}
	var in filterProbeInput

	for _, filters := range []map[string]string{{"owner_id": "not-a-uuid"}, {"tag": "vip"}} {
		fault, ok := Classify(set.Apply(&in, filters))

		if !ok || fault.Status != http.StatusUnprocessableEntity {
			t.Errorf("%v classified as %+v (ok=%v), want a 422 the caller can fix", filters, fault, ok)
		}
	}
}
