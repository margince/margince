// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSelectedGeneratedProseNeverSendsTheQuotedComponent(t *testing.T) {
	activity := note(crmcontracts.ActivityKindTask, "PRIVATE SOURCE QUOTATION", nil)
	activity.Subject = new(germanBody)
	activity.Version = new(crmcontracts.RowVersion(7))
	activity.SystemDisplay = &crmcontracts.SystemDisplay{GeneratedParts: &[]crmcontracts.SystemDisplayGeneratedParts{crmcontracts.SystemDisplayGeneratedPartsSubject}}
	lane := &recordingLane{reply: `{"already_in_target":false,"subject":"We talked about the renewal. They want to see the offer by Friday, and purchasing is no longer against the price.","body":""}`}
	now := clocktest.Now(t)
	svc := NewService(nil, oneActivity{activity}, func() time.Time { return now }).WithLane(lane)
	part, version := "subject", int64(7)
	got, err := svc.RenderPart(reader("en"), ids.From[ids.ActivityKind](ids.UUID(activity.Id)), nil, &part, &version)
	if err != nil {
		t.Fatal(err)
	}
	if len(lane.sent) != 1 || got.Subject == nil || got.Body != nil {
		t.Fatalf("unexpected rendering: %+v, requests %d", got, len(lane.sent))
	}
	payload, err := json.Marshal(lane.sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), *activity.Body) || !strings.Contains(string(payload), germanBody) {
		t.Fatal("selected request omitted generated prose or included the source quote")
	}
}

func TestAStaleOrUnmarkedGeneratedPartNeverReachesTheModel(t *testing.T) {
	activity := note(crmcontracts.ActivityKindTask, germanBody, nil)
	activity.Version = new(crmcontracts.RowVersion(7))
	lane := &recordingLane{reply: englishReply}
	now := clocktest.Now(t)
	svc := NewService(nil, oneActivity{activity}, func() time.Time { return now }).WithLane(lane)
	id := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
	part, version := "subject", int64(6)
	if _, err := svc.RenderPart(reader("en"), id, nil, &part, &version); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale version: %v", err)
	}
	version = 7
	if _, err := svc.RenderPart(reader("en"), id, nil, &part, &version); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unmarked part: %v", err)
	}
	if len(lane.sent) != 0 {
		t.Fatal("untrusted selection reached model")
	}
}

func TestSelectedGeneratedProseRequiresItsSourceVersion(t *testing.T) {
	activity := note(crmcontracts.ActivityKindTask, germanBody, nil)
	activity.SystemDisplay = &crmcontracts.SystemDisplay{GeneratedParts: &[]crmcontracts.SystemDisplayGeneratedParts{crmcontracts.SystemDisplayGeneratedPartsBody}}
	lane := &recordingLane{reply: englishReply}
	now := clocktest.Now(t)
	svc := NewService(nil, oneActivity{activity}, func() time.Time { return now }).WithLane(lane)
	part := "body"
	_, err := svc.RenderPart(reader("en"), ids.From[ids.ActivityKind](ids.UUID(activity.Id)), nil, &part, nil)
	if !errors.Is(err, apperrors.ErrVersionSkew) || len(lane.sent) != 0 {
		t.Fatalf("versionless component reached the model: %v, calls %d", err, len(lane.sent))
	}
}
