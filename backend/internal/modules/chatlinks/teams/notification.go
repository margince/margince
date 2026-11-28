// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"net/url"
	"strings"
)

// NotificationRoot accepts only notification resources belonging to the selected channel.
func NotificationRoot(resource, channel string) (string, bool, bool) {
	team, room, ok := strings.Cut(channel, "/")
	if !ok {
		return "", false, false
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(resource, "/"))
	if err != nil || strings.ContainsAny(decoded, "?#\\") {
		return "", false, false
	}
	rest := "teams/" + team + "/channels/" + room + "/messages/"
	odata := "teams('" + team + "')/channels('" + room + "')/messages('"
	if suffix, ok := strings.CutPrefix(decoded, rest); ok {
		return slashNotificationRoot(suffix)
	}
	if suffix, ok := strings.CutPrefix(decoded, odata); ok {
		return odataNotificationRoot(suffix)
	}
	return "", false, false
}

func slashNotificationRoot(suffix string) (string, bool, bool) {
	root, tail, reply := strings.Cut(suffix, "/replies/")
	if root == "" || strings.Contains(root, "/") || reply && (tail == "" || strings.Contains(tail, "/")) {
		return "", false, false
	}
	return root, !reply, true
}

func odataNotificationRoot(suffix string) (string, bool, bool) {
	root, tail, closed := strings.Cut(suffix, "')")
	if !closed || root == "" || strings.ContainsAny(root, "'/") {
		return "", false, false
	}
	if tail == "" {
		return root, true, true
	}
	reply, ok := strings.CutPrefix(tail, "/replies('")
	reply, closed = strings.CutSuffix(reply, "')")
	if !ok || !closed || reply == "" || strings.ContainsAny(reply, "'/") {
		return "", false, false
	}
	return root, false, true
}
