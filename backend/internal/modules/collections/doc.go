// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package collections owns the organizational surfaces over the core record
// types: lists — Live Lists (a stored filter) and Shortlists (chosen records)
// — saved views, and tags. Every client-supplied entity reference passes the
// visibility probe (H1), so a list cannot become a side channel onto rows the
// caller cannot read: a list is found by its sharing, and every member read,
// count and explanation applies the reader's own row scope.
//
// List lifecycle and Shortlist membership changes emit list.* events; tag
// and saved-view mutations stay audit-only, as the closed catalog defines no
// tag.* or saved_view.* types.
//
// Live Lists are checked on a schedule (liveevaluate.go): each check records
// who entered and left as list_member_event rows, and every read of them
// applies the reader's current row scope.
//
// Tables owned: list, list_member, list_member_event, list_revision,
// list_live_member, list_evaluation, list_visit, saved_view, tag, taggable,
// tag_suggestion, tag_suggestion_evidence.
// Imports shared + platform only; never a sibling module.
package collections
