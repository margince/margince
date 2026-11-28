// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// CheckServiceURL restricts Bot Framework delivery to supported service origins.
func CheckServiceURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || len(value) > 2048 || u.Scheme != httpsScheme || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || u.Path == "" {
		return messaging.ErrInvalid
	}
	host := u.Hostname()
	if host != "smba.trafficmanager.net" && host != "smba.infra.teams.microsoft.com" && !strings.HasSuffix(host, ".botapi.skype.com") {
		return messaging.ErrInvalid
	}
	return nil
}
