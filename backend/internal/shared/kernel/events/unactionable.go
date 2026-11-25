// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import "errors"

// ErrUnactionable is a handler saying this entry can never succeed, whatever
// the retry. The subject the event names is gone, so no later pass finds it.
//
// An entry wrapping it is acked with one loud log, as an undecodable entry is.
// Left pending, it would be reclaimed and logged on every cycle forever.
//
// A handler returns it only where the absence is settled. A race the consumer
// exists to fix returns the ordinary error, and is retried.
var ErrUnactionable = errors.New("events: the entry names a subject that is gone")
