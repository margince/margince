// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// fileRoutes are the GET routes whose link a browser tab can open, because each
// answers a file. TestEveryFileRouteIsOneTheContractServesAsAFile derives them.
var fileRoutes = map[string]bool{
	"/v1/agent-bundle":                                     true,
	"/v1/analytics/editions/{id}/export.csv":               true,
	"/v1/analytics/evaluate.csv":                           true,
	"/v1/attachments/{id}":                                 true,
	"/v1/companies/{id}/logo":                              true,
	"/v1/companies/{id}/logo/icon":                         true,
	"/v1/company/site-reads/{readId}/logo":                 true,
	"/v1/exports/bundle":                                   true,
	"/v1/finance/invoices/{id}/pdf":                        true,
	"/v1/forecast/shared/{token}/export.csv":               true,
	"/v1/imports/template":                                 true,
	"/v1/imports/{id}/problems.csv":                        true,
	"/v1/installation/jurisdiction/documents/{documentId}": true,
	"/v1/knowledge/documents/{id}":                         true,
	"/v1/legal-entities/{id}/logo/image":                   true,
	"/v1/offers/{id}/pdf":                                  true,
	"/v1/public/booking/company-logo":                      true,
	"/v1/public/rooms/documents/{documentId}/file":         true,
	"/v1/users/{id}/avatar":                                true,
}

// fileRefusalPage gives a file link opened in a tab a sentence when the file is
// gone or out of reach. A signed-in reader's language wins over the browser's.
func fileRefusalPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !fileRoutes[chiRoutePattern(r)] {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept")
		language := systemtext.Locale(r.Context())
		if !systemtext.HasLocale(r.Context()) {
			language = publicPreviewLocale(r)
		}
		page := httperr.RefusalPage{
			Lang:     string(language),
			Title:    systemtext.Copy("file.unavailable_title", language),
			Sentence: systemtext.Copy("file.unavailable_sentence", language),
		}
		next.ServeHTTP(w, r.WithContext(httperr.WithRefusalPage(r.Context(), page)))
	})
}
