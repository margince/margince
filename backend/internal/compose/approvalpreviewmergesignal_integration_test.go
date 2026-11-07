// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMergePreviewNamesEveryCarriedSignal(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			allowed := integration.AdminPerms
			allowed.Objects = maps.Clone(allowed.Objects)
			allowed.Objects["signal"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
			ctx := e.As(e.Rep1, nil, allowed)
			source, target := mergeContentPair(t, e, kind)
			store := signals.NewStore(e.DB(), nil)
			signal, err := store.CreateSignal(ctx, signals.CreateSignalInput{Kind: "other", SourceChannel: "derived", EntityType: &kind, EntityID: &source, Severity: "info", Summary: "Buyer asked for a review", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Signals) != 1 || preview.Signals[0].Id != signal.Id || preview.Signals[0].Summary != signal.Summary {
				t.Fatalf("signal scope=%+v", preview.Signals)
			}
			denied := integration.AdminPerms
			denied.Objects = maps.Clone(denied.Objects)
			delete(denied.Objects, "signal")
			hidden, refusal := e.Contacts.PreviewMergeSurvivorship(e.As(e.Rep1, nil, denied), kind, source, target, nil, nil)
			if !errors.Is(refusal, apperrors.ErrPermissionDenied) || hidden.Signals != nil {
				t.Fatalf("unauthorized signal scope=%+v error=%v", hidden.Signals, refusal)
			}
			e.Contacts.WithMergeSignalReader(nil)
			partial, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if !errors.Is(err, apperrors.ErrInvalidArgument) || partial.Signals != nil {
				t.Fatalf("unwired preview=%+v error=%v", partial, err)
			}
			e.Contacts.WithMergeSignalReader(signals.PreviewSignalCarry(signalSummaryWords))
			if kind == "contact" {
				_, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion)
			} else {
				_, err = e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion)
			}
			if err != nil {
				t.Fatal(err)
			}
			actual, err := store.GetSignal(ctx, ids.From[ids.SignalKind](ids.UUID(signal.Id)), storekit.IncludeArchived)
			if err != nil {
				t.Fatal(err)
			}
			if actual.EntityId == nil || ids.UUID(*actual.EntityId) != target || actual.Summary != signal.Summary {
				t.Fatalf("carried signal=%+v", actual)
			}
		})
	}
}
