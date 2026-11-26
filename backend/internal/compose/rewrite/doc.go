// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package rewrite is the one rewrite path over the words a rep has on screen
// (POST /rewrites). The email composer's rewrite verbs and Polish wording in
// Add task and Log activity both call it, and differ only in their envelope.
//
// The server owns each verb's instruction, and the prompt carries
// draftrules.Revise. The reply keeps the numbers, addresses, links and
// questions readerrendering.CheckSection holds, and the text's language. It
// reads no company context and no record, and nothing is stored.
package rewrite
