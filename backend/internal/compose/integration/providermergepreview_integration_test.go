// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestMergePreviewMatchesProviderPurchaseEffects(t *testing.T) {
	for _, tc := range []struct {
		name                                                          string
		sourceSubmitted, targetSubmitted, targetExists, extraCategory bool
		sourceAfter, targetAfter                                      provider.RunState
	}{
		{name: "unspent source", sourceAfter: provider.RunCancelled},
		{name: "both already submitted", sourceSubmitted: true, targetSubmitted: true, targetExists: true, sourceAfter: provider.RunInProgress, targetAfter: provider.RunInProgress},
		{name: "identical queued survivor", sourceSubmitted: true, targetExists: true, sourceAfter: provider.RunInProgress, targetAfter: provider.RunCancelled},
		{name: "overlapping queued survivor", sourceSubmitted: true, targetExists: true, extraCategory: true, sourceAfter: provider.RunInProgress, targetAfter: provider.RunSkipped},
		{name: "unspent source and submitted survivor", targetSubmitted: true, targetExists: true, sourceAfter: provider.RunCancelled, targetAfter: provider.RunInProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, store, source, target := providerMergeFixture(t)
			ctx := providerAdmin(e)
			queue := func(contact ids.UUID, submitted bool, cats []provider.Category) provider.Run {
				t.Helper()
				run, err := store.QueueRun(ctx, provider.QueueInput{ContactID: contact.String(), Provider: "surfe", Trigger: provider.TriggerManual, Categories: cats})
				if err != nil {
					t.Fatal(err)
				}
				if run.State != provider.RunQueued {
					t.Fatalf("queue refused: %+v", run)
				}
				if submitted {
					if err := store.ExecuteSubmit(ctx, run.ID); err != nil {
						t.Fatal(err)
					}
				}
				current, err := store.GetRun(ctx, contact.String(), run.ID)
				if err != nil {
					t.Fatal(err)
				}
				return current
			}
			sourceRun := queue(source, tc.sourceSubmitted, []provider.Category{"professional_email"})
			expected := map[string]provider.RunState{sourceRun.ID: tc.sourceAfter}
			if tc.targetExists {
				cats := []provider.Category{"professional_email"}
				if tc.extraCategory {
					cats = append(cats, "mobile")
				}
				targetRun := queue(target, tc.targetSubmitted, cats)
				expected[targetRun.ID] = tc.targetAfter
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if tc.targetExists && !tc.targetSubmitted {
				wantCount++
			}
			if len(preview.ProviderRuns) != wantCount {
				t.Fatalf("effects=%+v, want %d", preview.ProviderRuns, wantCount)
			}
			fingerprints := map[string]string{}
			for _, effect := range preview.ProviderRuns {
				_, fingerprints[effect.Before.ID] = runState(t, e, effect.Before.ID)
				if effect.AfterState != expected[effect.Before.ID] {
					t.Fatalf("wrong projected state: %+v", effect)
				}
				if len(effect.Before.Reservations) == 0 {
					t.Fatal("spending context missing")
				}
			}
			e.Contacts.WithProviderRunReader(nil)
			incomplete, refusal := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", source, target, nil, nil)
			if !errors.Is(refusal, apperrors.ErrInvalidArgument) || incomplete.ProviderRuns != nil || incomplete.Fields != nil {
				t.Fatalf("incomplete provider context: %+v %v", incomplete, refusal)
			}
			e.Contacts.WithProviderRunReader(integrations.ReadRunForContactTx)
			if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion); err != nil {
				t.Fatal(err)
			}
			for runID, state := range expected {
				actual, err := store.GetRun(ctx, target.String(), runID)
				if err != nil {
					t.Fatal(err)
				}
				if actual.State != state {
					t.Fatalf("run %s state=%s, want %s", runID, actual.State, state)
				}
			}
			for _, effect := range preview.ProviderRuns {
				actual, err := store.GetRun(ctx, target.String(), effect.Before.ID)
				if err != nil {
					t.Fatal(err)
				}
				_, fingerprint := runState(t, e, effect.Before.ID)
				if changed := fingerprints[effect.Before.ID] != fingerprint; changed != effect.FingerprintChanges {
					t.Errorf("fingerprint changed=%t, preview=%t", changed, effect.FingerprintChanges)
				}
				if !reflect.DeepEqual(actual.Reservations, effect.Before.Reservations) || actual.SkipReason != effect.AfterSkipReason {
					t.Fatalf("different purchase outcome: %+v versus %+v", actual, effect)
				}
			}
		})
	}
}

func providerMergeFixture(t *testing.T) (*Env, *integrations.Store, ids.UUID, ids.UUID) {
	t.Helper()
	e := Setup(t)
	now := func() time.Time { return time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC) }
	reg, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, now))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := integrations.NewStore(e.DB(), keyvault.NewMemory(), reg, now)
	if err != nil {
		t.Fatal(err)
	}
	store := compose.BindProviderDomain(raw).WithSubmitEnqueue(func(context.Context, pgx.Tx, string, string) error { return nil })
	permissions := AdminPerms
	permissions.Objects = providerAdminObjects()
	grant := permissions.Objects["integrations"]
	grant.Create = true
	permissions.Objects["integrations"] = grant
	if _, err := store.Connect(e.As(e.AdminUser, nil, permissions), integrations.ConnectInput{Provider: "surfe", APIKey: "synthetic-test-key", Config: &integrations.ConfigInput{Mode: "on_demand", Preset: "full", Categories: []string{"professional_email", "mobile"}}}); err != nil {
		t.Fatal(err)
	}
	var people []ids.UUID
	for range 2 {
		person, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Merge Person", Source: "manual", Social: map[string]any{"linkedin": "https://www.linkedin.com/in/merge-person"}})
		if err != nil {
			t.Fatal(err)
		}
		people = append(people, ids.UUID(person.Id))
	}
	return e, store, people[0], people[1]
}
