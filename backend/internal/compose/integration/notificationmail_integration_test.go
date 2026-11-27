// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The settings read says whether the installation's own relay is wired, the
// one that sends the approval message and the morning digest.
func TestTheNotificationSettingsSayWhetherEmailLeaves(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []compose.Option
		want bool
	}{
		{"no relay", nil, false},
		{"a relay", []compose.Option{compose.WithOperatorMail(apptest.DiscardingMailer{})}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := apptest.SetupAppWithOptions(t, tc.opts...)
			apptest.BootstrapWorkspaceSession(t, e, "Notices", "admin@notices.test", "Admin")
			var list struct {
				EmailDeliveryAvailable *bool `json:"email_delivery_available"`
			}
			if status := e.Call(t, "GET", "/v1/me/notification-preferences", nil, nil, &list); status != http.StatusOK {
				t.Fatalf("reading the notification settings → %d", status)
			}
			if list.EmailDeliveryAvailable == nil || *list.EmailDeliveryAvailable != tc.want {
				t.Errorf("email_delivery_available = %v, want %v", list.EmailDeliveryAvailable, tc.want)
			}
		})
	}
}
