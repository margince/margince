// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A host with no connection is answered the way a refused connection is, by
// every door the seam has, so the scheduling module maps one sentinel.
func TestSchedulingCalendarWithoutAConnectionReadsAsRefused(t *testing.T) {
	e := integration.Setup(t)
	calendar := newSchedulingCalendar(e.Pool, capture.NewRegistry(e.DB(), nil, nil, nil))
	host := ids.From[ids.UserKind](e.AdminUser)
	now := time.Now()

	_, listErr := calendar.List(e.Admin(), host, "bogus")
	_, busyErr := calendar.Busy(e.Admin(), host, "gcal", "primary", now, now.Add(time.Hour))

	for name, err := range map[string]error{"list": listErr, "busy": busyErr} {
		if !errors.Is(err, connector.ErrAuthRejected) {
			t.Errorf("%s answered %v, want the refused-connection sentinel", name, err)
		}
	}
}
