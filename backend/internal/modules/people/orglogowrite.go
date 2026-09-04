// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The one statement that moves a company's mark.
//
// Four writers reach the pair of logo columns — a person setting the
// installation's own mark, the same person taking it off, a website resolve, and
// a site-read confirmation adopting what it found — and each carried its own
// copy of this UPDATE. They agreed, which is the state a set of copies is in
// right up until one of them is edited.
//
// CLEARING IS SETTING THE MARK TO NOTHING, so the clear takes this statement too
// with two NULL values rather than a statement of its own. What made the fourth
// copy look necessary was the SET list, and a bind parameter answers that.
//
// The RETURNING is the load-bearing half and the reason no caller may write its
// own: the pre-write key comes back from the statement that superseded it, so
// the caller can collect the object nothing references any more. Read separately
// afterwards it would name whatever the NEXT write had since put there, and the
// bytes still in use would be the ones collected.
//
// `archived_at IS NULL` is here rather than at each call site for the same
// reason: a row visible when the write was authorized can be archived before the
// write lands, and pgx.ErrNoRows is how every caller learns that happened.
const orgLogoWrite = `UPDATE organization SET logo_object_key = $2, logo_origin = $3
	WHERE id = $1 AND archived_at IS NULL
	RETURNING (SELECT o.logo_object_key FROM organization o WHERE o.id = $1),
	          (SELECT o.logo_origin FROM organization o WHERE o.id = $1)`
