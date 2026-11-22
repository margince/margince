// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"
)

func TestOnlyTheReadersOwnListJudgesDueDatesInTheirZone(t *testing.T) {
	hcm, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	for scope, want := range map[TaskScope]string{
		TasksMine:       "Asia/Ho_Chi_Minh",
		TasksVisible:    "",
		TasksOwnedBy:    "",
		TasksUnassigned: "",
	} {
		if got := dayZoneOf(scope, hcm); got != want {
			t.Errorf("scope %v judges due dates in %q, want %q", scope, got, want)
		}
	}
}

func TestTheReadersOwnListMeasuresItsDayInThePersonalZone(t *testing.T) {
	berlin, hcm := time.FixedZone("Berlin", 2*3600), time.FixedZone("Ho Chi Minh", 7*3600)
	svc := &Service{
		zone:         func(context.Context) (*time.Location, error) { return berlin, nil },
		personalZone: func(context.Context) (*time.Location, error) { return hcm, nil },
	}
	if got, err := svc.location(context.Background()); err != nil || got != berlin {
		t.Fatalf("a combined read measures the day in %v (%v), want the installation's zone", got, err)
	}
	if got, err := svc.forReader().location(context.Background()); err != nil || got != hcm {
		t.Fatalf("the reader's own list measures the day in %v (%v), want their zone", got, err)
	}
	unbound := &Service{zone: svc.zone}
	if got, err := unbound.forReader().location(context.Background()); err != nil || got != berlin {
		t.Fatalf("with no personal zone the reader falls back to %v (%v), want the installation's", got, err)
	}
}
