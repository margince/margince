// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAutomaticLogDoesNotInventTheDefaultCompany(t *testing.T) {
	for _, actor := range []principal.Principal{
		{Type: principal.PrincipalSystem, ID: "system:fixture"},
		{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: ids.NewV7()},
	} {
		t.Run(actor.ID, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), actor)
			got, err := loggedCorrespondenceEntity(ctx, nil, LogActivityInput{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != nil {
				t.Fatalf("automatic work acquired company %s", got)
			}
		})
	}
}

func TestAQueuedLogKeepsItsRecordedCompanyWithoutResolvingItsWorker(t *testing.T) {
	entity := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:delivery"})
	input := LogActivityInput{}
	input.correspondenceEntity = entity
	got, err := loggedCorrespondenceEntity(ctx, nil, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != entity {
		t.Fatalf("queued company = %v, want %s", got, entity)
	}
}
