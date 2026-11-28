// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import "testing"

func TestNotificationRootPreservesItsExactChannelAndThread(t *testing.T) {
	for _, test := range []struct {
		resource, root string
		isRoot, ok     bool
	}{
		{"teams/T/channels/19:room@thread.tacv2/messages/ROOT", "ROOT", true, true},
		{"/teams/T/channels/19%3Aroom%40thread.tacv2/messages/ROOT/replies/REPLY", "ROOT", false, true},
		{"teams('T')/channels('19:room@thread.tacv2')/messages('ROOT')", "ROOT", true, true},
		{"teams('T')/channels('19:room@thread.tacv2')/messages('ROOT')/replies('REPLY')", "ROOT", false, true},
		{"teams/T/channels/19:other@thread.tacv2/messages/ROOT", "", false, false},
		{"teams/OTHER/channels/19:room@thread.tacv2/messages/ROOT", "", false, false},
		{"teams/T/channels/19:room@thread.tacv2/messages/ROOT/replies/", "", false, false},
		{"teams/T/channels/19:room@thread.tacv2/messages/ROOT?other=1", "", false, false},
		{"https://graph.microsoft.com/v1.0/teams/T/channels/19:room@thread.tacv2/messages/ROOT", "", false, false},
		{"teams('T')/channels('19:room@thread.tacv2')/messages('ROOT')/replies('')", "", false, false},
	} {
		t.Run(test.resource, func(t *testing.T) {
			root, isRoot, ok := NotificationRoot(test.resource, "T/19:room@thread.tacv2")
			if root != test.root || isRoot != test.isRoot || ok != test.ok {
				t.Fatalf("root=%q isRoot=%v ok=%v", root, isRoot, ok)
			}
		})
	}
}
