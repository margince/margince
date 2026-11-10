// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"golang.org/x/text/language"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func publicPreviewLocale(r *http.Request) textlang.Lang {
	if explicit := r.URL.Query().Get("locale"); textlang.Known(explicit) {
		return textlang.Lang(explicit)
	}
	if explicit := r.Header.Get("X-Margince-Display-Locale"); textlang.Known(explicit) {
		return textlang.Lang(explicit)
	}
	header := r.Header.Get("Accept-Language")
	if len(header) > 4096 {
		return textlang.English
	}
	preferred, _, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return textlang.English
	}
	supported := make([]language.Tag, len(textlang.Shipped))
	for i, locale := range textlang.Shipped {
		supported[i] = language.Make(string(locale))
	}
	_, index, _ := language.NewMatcher(supported).Match(preferred...)
	return textlang.Shipped[index]
}
