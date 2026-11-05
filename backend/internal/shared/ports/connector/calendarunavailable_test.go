// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAnUnavailableCalendarIsTheSharedNoUsableAnswerAndKeepsItsCause(t *testing.T) {
	cause := errors.New("provider status 503")
	err := NewCalendarUnavailable("team@example.test", "notFound", cause)

	if !errors.Is(err, apperrors.ErrProviderUnusable) || !errors.Is(err, cause) {
		t.Fatalf("%v must classify as the shared sentinel and still unwrap to its cause", err)
	}
	for _, want := range []string{"team@example.test", "notFound", "provider status 503"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the log line %q does not carry %q", err.Error(), want)
		}
	}
	bare := NewCalendarUnavailable("primary", "", nil)
	if strings.Contains(bare.Error(), "()") || bare.Unwrap() != nil {
		t.Errorf("a failure the provider did not name rendered as %q", bare.Error())
	}
}

func TestOnlyAnUnusableProviderAnswerBecomesAnUnavailableCalendar(t *testing.T) {
	already := NewCalendarUnavailable("a", "notFound", nil)
	for name, tc := range map[string]struct {
		err         error
		unavailable bool
	}{
		"an unreadable body":     {errors.New("calendar: invalid response"), true},
		"a refused credential":   {ErrAuthRejected, false},
		"an unreachable service": {ErrUnreachable, false},
		"a cancelled request":    {context.Canceled, false},
		"a deadline":             {context.DeadlineExceeded, false},
		"already classified":     {already, true},
	} {
		got := CalendarReadFailed("a", tc.err)
		_, is := errors.AsType[*CalendarUnavailableError](got)
		if is != tc.unavailable || (!tc.unavailable && got != tc.err) {
			t.Errorf("%s → %v (unavailable=%v), want unavailable=%v", name, got, is, tc.unavailable)
		}
	}
	if CalendarReadFailed("a", nil) != nil {
		t.Error("no failure was invented")
	}
	if got := CalendarReadFailed("a", already); got != error(already) {
		t.Error("an already classified failure was wrapped a second time")
	}
}

func TestTheClientSentenceNamesNeitherTheCalendarNorTheProvidersReason(t *testing.T) {
	code, message := NewCalendarUnavailable("team@example.test", "notFound", nil).MessageFault()
	if code != CalendarUnavailableCode || strings.Contains(message, "team@example.test") || strings.Contains(message, "notFound") {
		t.Errorf("code=%q message=%q", code, message)
	}
}
