// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package taskexplain writes why one open task is on its reader's list. It
// says what happened, what it costs if nothing is done, and the next step.
//
// It is the contact brief turned on a task. The input is the task, the message
// it came from, and the contact brief's own projection of the relationship, all
// read as the caller. So it can only say what that caller could open, and the
// task and the contact page never tell two stories about one contact.
//
// A sentence's nature carries its section: a fact is what happened, an
// assessment what is at stake, a recommendation what to do. Every sentence
// cites the records it came from or is dropped, and a stock sentence that
// would fit any task is refused.
//
// Cached per reader in generatedsummary on a fingerprint of the input, and
// rewritten before the answer once that moves. With no model, the floor states
// the facts alone and leaves stakes and the next step unsaid.
package taskexplain
