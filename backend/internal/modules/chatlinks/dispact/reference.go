// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"strconv"
	"strings"
)

// MessageSequence accepts legacy sequence references only within their selected channel.
func MessageSequence(reference, channel string, wholeThread bool) (int64, bool) {
	ref := strings.TrimSpace(reference)
	if inner, ok := strings.CutPrefix(ref, "<m:"); ok {
		ref, _, _ = strings.Cut(strings.TrimSuffix(inner, ">"), "|")
	}
	if name, number, found := strings.Cut(ref, ":"); found {
		if !strings.EqualFold(strings.TrimPrefix(name, "#"), channel) {
			return 0, false
		}
		root, reply, hasReply := strings.Cut(number, ":")
		ref = root
		if hasReply && !wholeThread {
			ref = reply
		}
	}
	seq, err := strconv.ParseInt(ref, 10, 64)
	return seq, err == nil && seq > 0 && seq <= 9_007_199_254_740_991
}
