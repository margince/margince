// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// Booking scenarios use real OAuth, registry, consent and scheduling wiring;
// only Google's HTTP boundary is replaced.
func setupBookingApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e, _ := setupBookingProvider(t)
	return e
}

func setupBookingProvider(t *testing.T) (*apptest.AppEnv, *apptest.BookingProvider) {
	t.Helper()
	previous := http.DefaultTransport
	provider := apptest.NewBookingProvider(previous)
	http.DefaultTransport = provider
	t.Cleanup(func() { http.DefaultTransport = previous })
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault), compose.WithGmailCapture(compose.GmailConfig{
		ClientID: "calendar-fixture", ClientSecret: "calendar-fixture", StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://mail.example.test",
	}, compose.CaptureConfig{}))
	e.Vault = vault
	return e, provider
}

// startBookingWorker boots the composed job runner with Google registered behind
// the fixture's transport, so calendar delivery runs through the real gcal adapter.
func startBookingWorker(t *testing.T, e *apptest.AppEnv) (*jobs.Runner, <-chan *river.Event, <-chan *river.Event) {
	t.Helper()
	db := compose.InstallationDB(e.Pool)
	registry := capture.NewRegistry(db, capture.NewSink(db), identity.NewServiceFor(db), e.Vault)
	registry.Register(gcal.New(gcal.NewOAuth(gcal.OAuthConfig{ClientID: "calendar-fixture", ClientSecret: "calendar-fixture"}), gcal.NewAPI(nil, "")))
	return jobtest.StartTestJobRunner(t, e.Pool, compose.JobRunnerConfig{GmailRegistry: registry, ControllerVault: e.Vault, SendOrigin: compose.SendOrigin{PublicBaseURL: "https://mail.example.test"}})
}
