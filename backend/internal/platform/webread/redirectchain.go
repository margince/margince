// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"net/http"
	"slices"
)

func responseChain(request *http.Request) []string {
	var chain []string
	for request != nil {
		chain = append(chain, request.URL.String())
		if request.Response == nil {
			break
		}
		request = request.Response.Request
	}
	slices.Reverse(chain)
	return chain
}
