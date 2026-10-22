// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A room's parts read back on the record seam by their own ids, as the room
// carries them. An archive staged on one can then name what it archives. An id
// no room holds is absent.
func TestADealRoomsPartsReadBackOnTheSeam(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	var thread struct {
		Comments []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"comments"`
	}
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/threads", AnyMap{
		"body": "Any questions on the terms?", "source": "manual",
	}, nil, &thread); status != http.StatusCreated || len(thread.Comments) == 0 {
		t.Fatalf("opening a thread = %d with %d comment(s)", status, len(thread.Comments))
	}
	var participants struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID+"/participants", nil, nil, &participants); status != http.StatusOK || len(participants.Data) == 0 {
		t.Fatalf("listing participants = %d with %d row(s)", status, len(participants.Data))
	}

	seam := compose.NewProviderFor(e.DB())
	reader := roomReader(t, e)
	for entity, id := range map[datasource.EntityType]string{
		datasource.EntityDealRoomComment:     thread.Comments[0].ID,
		datasource.EntityDealRoomParticipant: participants.Data[0].ID,
	} {
		record, err := seam.Read(reader, datasource.EntityRef{Type: entity, ID: ids.MustParse(id)})
		if err != nil || record.Ref.ID.String() != id {
			t.Errorf("reading %s %s on the seam answered %+v (%v)", entity, id, record.Ref, err)
		}
		if _, err := seam.Read(reader, datasource.EntityRef{Type: entity, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("reading a %s no room holds answered %v, want not found", entity, err)
		}
	}
}

// roomReader is a seller who may read every deal room.
func roomReader(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	user := ids.NewV7()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}, "deal_room": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}
