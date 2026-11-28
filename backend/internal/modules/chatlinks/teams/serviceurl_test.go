// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import "testing"

func TestConnectorServiceURLDoesNotSendApplicationTokensElsewhere(t *testing.T) {
	for _, value := range []string{"https://smba.trafficmanager.net/teams/", "https://smba.trafficmanager.net/emea/", "https://smba.infra.teams.microsoft.com/teams/", "https://amer.botapi.skype.com/amer/"} {
		if err := CheckServiceURL(value); err != nil {
			t.Fatalf("valid Microsoft endpoint refused: %q %v", value, err)
		}
	}
	for _, value := range []string{"http://smba.trafficmanager.net/teams/", "https://other.test/teams/", "https://smba.trafficmanager.net.evil.test/teams/", "https://botapi.skype.com.evil.test/teams/", "https://user@smba.trafficmanager.net/teams/", "https://smba.trafficmanager.net:444/teams/", "https://smba.trafficmanager.net/teams/?redirect=elsewhere", "https://smba.trafficmanager.net/teams/#fragment", "https://127.0.0.1/teams/"} {
		if err := CheckServiceURL(value); err == nil {
			t.Fatalf("unsafe credential destination accepted: %q", value)
		}
	}
}
