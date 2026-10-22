// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import "errors"

// ErrUnactionable is a handler saying this entry can never succeed, whatever
// the retry. The subject the event names is gone, so no later pass finds it.
//
// An entry wrapping it is acked with a loud log. That is the answer an
// undecodable entry gets, for the same reason: left pending, it is reclaimed
// every cycle forever.
//
// Eleven such entries on one staging installation retried every five minutes
// for hours. That noise hides a lane about to recover.
//
// A handler returns it only where the absence is settled. A race the consumer
// exists to fix returns the ordinary error, and is retried.
var ErrUnactionable = errors.New("events: the entry names a subject that is gone")
