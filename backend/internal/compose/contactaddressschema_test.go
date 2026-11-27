// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

//craft:ignore naked-any JSON Schema validates decoded request bodies
func TestContactAddressContractAdmitsLegacyReplacementAndBoundsCreation(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		path := "/contacts"
		if method == http.MethodPatch {
			path += "/{id}"
		}
		operation := doc.Paths.Value(path).GetOperation(method)
		httpSchema := operation.RequestBody.Value.Content.Get("application/json").Schema.Value
		for _, size := range []int{values.MaxEmailLength, values.MaxEmailLength + 1} {
			t.Run(method+"/"+strconv.Itoa(size), func(t *testing.T) {
				address := strings.Repeat("a", size-len("@legacy.test")) + "@legacy.test"
				body := map[string]any{
					"full_name": "Legacy contact", "source": "manual",
					"emails": []any{map[string]any{"email": address}},
				}
				err := httpSchema.VisitJSON(body)
				refused := method == http.MethodPost && size > values.MaxEmailLength
				if (err != nil) != refused {
					t.Errorf("%s address length %d: schema error %v, want refused %t", method, size, err, refused)
				}
			})
		}
	}
}
