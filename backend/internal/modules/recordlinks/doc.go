// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordlinks owns the typed link between two core records: a follow-up
// deal, a renewal, a company tied to a deal. A link joins any two of deal, company and
// contact. Its type is one of core's words or an extension's `ext:<unit>:<type>`,
// both looked up in the registry the record kinds live in.
//
// A link is gated on both records. The caller updates the record it starts on
// and may attach to the one it ends on. A read drops a link whose other end the
// caller may not open. Archiving an end writes nothing here: the read hides the
// link while the end is archived.
//
// Tables owned: docs/reference/entity-model/recordlinks.md.
package recordlinks
