// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A prepared answer goes to the reader who asked and is stored nowhere. So a
// German and an English reader on one installation get different languages.
func TestAPreparedAnswerIsWrittenInTheReadersOwnLanguage(t *testing.T) {
	svc := &Service{}
	systems := map[string]string{}
	for _, locale := range []string{"de", "en"} {
		ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Locale: locale})
		lang := svc.answerLanguage(ctx)
		if lang != locale {
			t.Fatalf("a reader who reads %s is answered in %q", locale, lang)
		}
		req := AskRequest(crmcontracts.CompanyQuestionWhatsOpen, askInput().WordedIn(lang), lang)
		if !strings.Contains(req.System, promptlang.Rule(locale)) {
			t.Errorf("the %s reader's request does not carry the %s language rule", locale, locale)
		}
		systems[locale] = req.System
	}
	if strings.Contains(systems["de"], promptlang.Rule("en")) {
		t.Error("the German reader's request still asks for English")
	}
}
