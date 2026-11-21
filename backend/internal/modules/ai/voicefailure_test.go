// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestVoiceFailuresTranslateWithoutExposingProviderText(t *testing.T) {
	langcopytest.Census(t, voiceFailureWords)
	causes := []error{nil, ErrProviderQuota, ErrUnconfiguredModel, ErrProviderThrottled, errProviderRefused, model.ErrOutputWithheld, model.ErrRequestRejected, errors.New("no bound provider"), errors.New("invalid response"), errors.New("voice build needs at least private-provider-response")}
	for _, cause := range causes {
		for _, language := range []textlang.Lang{textlang.German, textlang.Vietnamese} {
			wrapped := cause
			if cause != nil {
				wrapped = fmt.Errorf("private-provider-response: %w", cause)
			}
			got := SafeVoiceBuildFailure(wrapped, language)
			if got == "" || got == SafeVoiceBuildFailure(wrapped, textlang.English) || strings.Contains(got, "private-provider-response") {
				t.Fatalf("%s failure is untranslated or leaks provider text: %s", language, got)
			}
		}
	}
}

func TestVoiceWordFloorPreservesMeasuredCounts(t *testing.T) {
	err := fmt.Errorf("build: %w", &voiceWordFloorError{words: 123})
	for _, language := range []textlang.Lang{textlang.English, textlang.German, textlang.Vietnamese} {
		got := SafeVoiceBuildFailure(err, language)
		if !strings.Contains(got, fmt.Sprint(StarterVoiceWords)) || !strings.Contains(got, "123") {
			t.Fatalf("%s lost required or measured count: %s", language, got)
		}
	}
}

// Every member reads the unconfigured text, and most cannot open the AI
// settings. So it names who fixes it and where, in words a reader knows.
func TestUnconfiguredVoiceFailureNamesTheAdministratorAndNoInternals(t *testing.T) {
	internals := []string{"stand-in", "tier", "bind", "Ersatzmodell", "Stufe", "zuordnen", "Ordne", "thay thế ngoại tuyến", "từng cấp"}
	path := map[textlang.Lang]string{
		textlang.English:    "Settings → AI models → Providers",
		textlang.German:     "Einstellungen → KI-Modelle → Anbieter",
		textlang.Vietnamese: "Cài đặt → Mô hình AI → Nhà cung cấp",
	}
	for language, where := range path {
		got := SafeVoiceBuildFailure(fmt.Errorf("build: %w", ErrUnconfiguredModel), language)
		if !strings.Contains(got, where) {
			t.Fatalf("%s text does not name %q: %s", language, where, got)
		}
		for _, word := range internals {
			if strings.Contains(got, word) {
				t.Fatalf("%s text carries the internal term %q: %s", language, word, got)
			}
		}
	}
}
