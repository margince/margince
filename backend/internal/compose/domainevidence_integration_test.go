// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestDomainTriageStoresBaseLanguageAroundUnchangedEvidence(t *testing.T) {
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			e := integration.Setup(t)
			integration.SetBaseLanguage(e.Admin(), t, e.Pool, string(lang))
			site := &fakeSite{pages: map[string]fakeSitePage{seedURL: {text: readable("Sebastian Kestner.") + " This is Sebastian's personal website."}}}
			triage := &triageBrainFake{verdict: `{"kind":"personal","confidence":0.95,"reason":"Original source words: unverändert."}`}
			args := openTriageQuestion(t, e, triageTestDomain, "sebastian@"+triageTestDomain, "Sebastian Kestner")
			worker := newTriageTestWorker(e, site, &triageBrainFake{}, triage)
			if err := worker.run(e.As(e.Rep1, nil, integration.AdminPerms), args); err != nil {
				t.Fatal(err)
			}
			stored := e.WsScalar(t, `SELECT evidence FROM company_domain_disposition WHERE domain=$1`, triageTestDomain)
			said := domainEvidenceCopyFor(lang)
			if !strings.HasSuffix(stored, "Original source words: unverändert.") || !strings.Contains(stored, said.Say(domainEvidence.Personal)) {
				t.Fatalf("stored %s evidence lost its language or source: %s", lang, stored)
			}
			integration.SetBaseLanguage(e.Admin(), t, e.Pool, "en")
			audits := e.WsCount(t, `SELECT count(*) FROM audit_log`)
			if err := worker.run(e.As(e.Rep1, nil, integration.AdminPerms), args); err != nil {
				t.Fatal(err)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log`); got != audits {
				t.Fatal("replaying settled work after a language change wrote another audit")
			}
			if got := e.WsScalar(t, `SELECT evidence FROM company_domain_disposition WHERE domain=$1`, triageTestDomain); got != stored {
				t.Fatal("changing language rewrote stored evidence")
			}
		})
	}
}
