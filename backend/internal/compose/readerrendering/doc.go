// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package readerrendering shows one note, email, meeting or call in the
// reader's own language when that reader asks for it
// (POST /activities/{id}/reader-rendering).
//
// The rendering is for one reader, so it is written in their language. It is
// stored nowhere: no copy, no audit row, no event, no payload. Text the whole
// team shares keeps the base language (docs/explanation/shared-record-language.md).
//
// The text is read through activities.Store.GetActivityContent. So a message
// outside the caller's audience, a held row and an archived row answer 404
// before a model sees a word of it.
package readerrendering
