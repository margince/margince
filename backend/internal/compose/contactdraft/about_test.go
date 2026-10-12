// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// usableActivities answers the activities it holds and not-found for any
// other, the answer the content gate gives.
type usableActivities map[ids.ActivityID]crmcontracts.Activity

func (u usableActivities) GetActivityContent(_ context.Context, id ids.ActivityID, _ storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	activity, ok := u[id]
	if !ok {
		return crmcontracts.Activity{}, apperrors.ErrNotFound
	}
	return activity, nil
}

// recordingLane keeps what it was asked and answers nothing, so the draft
// falls to its floor after the request was built.
type recordingLane struct{ asked []model.Request }

func (l *recordingLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.asked = append(l.asked, req)
	return model.Response{}, errors.New("recorded, not answered")
}

func draftAbout(t *testing.T, lane *recordingLane, usable usableActivities, about ids.ActivityID) *httptest.ResponseRecorder {
	t.Helper()
	pathID, path := contactAt("lead@customer.test")
	svc := contactdraft.NewService(pathContact{path}, lane).WithAboutActivities(usable)
	seat := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	req := httptest.NewRequestWithContext(human, http.MethodPost, "/contacts/x/draft-email",
		strings.NewReader(fmt.Sprintf(`{"about_activity_id":%q}`, about.String())))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	contactdraft.NewHandlers(svc).DraftContactEmail(rec, req, openapi_types.UUID(pathID.UUID))
	return rec
}

func TestTheMeetingADraftFollowsUpLeadsWhatTheWriterReads(t *testing.T) {
	t.Parallel()
	meeting, deal := ids.From[ids.ActivityKind](ids.NewV7()), ids.NewV7()
	subject := "Pricing review"
	links := []crmcontracts.ActivityLink{{EntityType: crmcontracts.ActivityLinkEntityTypeDeal, EntityId: openapi_types.UUID(deal)}}
	lane := &recordingLane{}
	rec := draftAbout(t, lane, usableActivities{meeting: {
		Id: openapi_types.UUID(meeting.UUID), Kind: crmcontracts.ActivityKindMeeting, Subject: &subject,
		OccurredAt: clocktest.Now(t).Add(-72 * time.Hour), Links: &links,
	}}, meeting)
	if rec.Code != http.StatusOK || len(lane.asked) == 0 {
		t.Fatalf("drafting about a meeting answered %d and asked the writer %d times: %s", rec.Code, len(lane.asked), rec.Body.String())
	}
	var asked strings.Builder
	for _, message := range lane.asked[0].Messages {
		fmt.Fprint(&asked, message)
	}
	for _, want := range []string{`"about"`, subject, deal.String()} {
		if !strings.Contains(asked.String(), want) {
			t.Errorf("the writer was not given %s: %s", want, asked.String())
		}
	}
}

func TestADraftAboutAnActivityTheCallerCannotUseIsRefused(t *testing.T) {
	t.Parallel()
	rec := draftAbout(t, &recordingLane{}, usableActivities{}, ids.From[ids.ActivityKind](ids.NewV7()))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "about_activity_id") {
		t.Fatalf("drafting about an unusable activity answered %d: %s, want 422 on about_activity_id", rec.Code, rec.Body.String())
	}
}
