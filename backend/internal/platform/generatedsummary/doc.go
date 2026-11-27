// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package generatedsummary stores the generated summaries of a company, a
// contact or a deal, one row per reader, kind and record.
//
// Tables owned: docs/reference/entity-model/platform.md.
//
// A row is derived content: it has no audit row and no outbox event, and a
// rebuild from the record replaces it.
package generatedsummary
