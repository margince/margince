// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// revokingStore opens a document and, while it is open, has the seller revoke
// the buyer's seat, so the download's bookkeeping is refused after the open.
type revokingStore struct {
	blobstore.Store
	revoke func()
	closed bool
}

func (s *revokingStore) Get(context.Context, string) (io.ReadCloser, blobstore.Object, error) {
	s.revoke()
	return closeRecorder{Reader: strings.NewReader("proposal bytes"), closed: &s.closed}, blobstore.Object{}, nil
}

type closeRecorder struct {
	io.Reader
	closed *bool
}

func (c closeRecorder) Close() error {
	*c.closed = true
	return nil
}

// A download whose bookkeeping is refused closes the reader it opened.
func TestARefusedDownloadClosesTheDocumentItOpened(t *testing.T) {
	e := Setup(t)
	buyer := seedBuyerInARoom(t, e, "download.revoked@acme.test")
	doc := roomDocument(t, e, buyer)
	sess := signedInBuyer(t, e, buyer)
	seller := e.As(e.AdminUser, nil, roomErasureAdmin)
	store := &revokingStore{Store: blobstore.NewMemory(), revoke: func() {
		if _, err := dealrooms.NewStore(e.DB()).RevokeParticipant(seller, buyer.room, buyer.seat); err != nil {
			t.Errorf("revoking the buyer's seat: %v", err)
		}
	}}
	ctx := dealrooms.WithSession(principal.WithWorkspaceID(context.Background(), e.WS), sess)
	ctx = principal.WithActor(ctx, dealrooms.BuyerPrincipal(sess.ParticipantID))
	w := httptest.NewRecorder()

	dealrooms.NewHandlers(e.DB()).WithDocumentStore(store).DownloadBuyerRoomDocument(
		w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx), openapi_types.UUID(doc))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a seat revoked during the download", w.Code)
	}
	if !store.closed {
		t.Error("the refused download left the document reader open")
	}
}
