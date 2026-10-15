// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

func captureFrom(t *testing.T, body string) (CaptureConfig, string) {
	t.Helper()
	var out bytes.Buffer
	cfg := CaptureConfigFromDeploy(configtest.MustLoad(t, body, captureSettings()...), slog.New(slog.NewTextHandler(&out, nil)))
	return cfg, out.String()
}

// The trace names senders unless an operator wrote otherwise: a page of
// decisions naming nobody cannot answer why a message did not arrive. A file
// that never mentions the key says nothing, which is not saying no.
func TestTracePayloadsIsOnUnlessTheFileTurnsItOff(t *testing.T) {
	for body, want := range map[string]bool{
		"version: 1\n":                        true,
		"capture:\n  trace_payloads: true\n":  true,
		"capture:\n  trace_payloads: false\n": false,
	} {
		if got, _ := captureFrom(t, body); got.TracePayloads != want {
			t.Errorf("%q: TracePayloads = %v, want %v", body, got.TracePayloads, want)
		}
	}
}

func TestReservedDomainProposalsAreSkippedUnlessTheFileSaysOtherwise(t *testing.T) {
	if got, _ := captureFrom(t, "version: 1\n"); !got.SkipReservedDomainProposals {
		t.Error("a silent file proposes reserved-domain senders, want them skipped")
	}
	if got, _ := captureFrom(t, "capture:\n  skip_reserved_domain_proposals: false\n"); got.SkipReservedDomainProposals {
		t.Error("an explicit false still skips reserved-domain senders")
	}
}

func TestBackfillCeilingDefaultsToNoCapAndReadsTheOperatorsNumber(t *testing.T) {
	if got, _ := captureFrom(t, "version: 1\n"); got.MaxBackfillMonths != 0 {
		t.Errorf("a silent file caps the backfill at %d months, want no cap", got.MaxBackfillMonths)
	}
	if got, _ := captureFrom(t, "capture:\n  max_backfill_months: 24\n"); got.MaxBackfillMonths != 24 {
		t.Errorf("MaxBackfillMonths = %d, want 24", got.MaxBackfillMonths)
	}
}

// The moved keys still load, since deleting them would refuse an upgrade's
// boot, so the warning is the one thing that says they no longer act. It names
// where the list moved, and an operator who never set them hears nothing.
func TestCaptureWarnsOnlyAboutSettingsItNoLongerActsOn(t *testing.T) {
	for _, body := range []string{"version: 1\n", "capture:\n  transactional_extra: [esp.example]\n"} {
		if _, log := captureFrom(t, body); log != "" {
			t.Errorf("%q: warned %q, want silence", body, log)
		}
	}
	for _, body := range []string{
		"capture:\n  freemail_extra: [provider.example]\n",
		"capture:\n  freemail_never: [customer.example]\n",
		"capture:\n  freemail_extra: [a.example]\n  freemail_never: [b.example]\n",
	} {
		_, log := captureFrom(t, body)
		if strings.Count(log, "level=WARN") != 1 || !strings.Contains(log, "consumer-mail-domains") || !strings.Contains(log, "margince.yaml") {
			t.Errorf("%q: warned %q, want one warning naming the new surface and the file to edit", body, log)
		}
	}
}
