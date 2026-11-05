// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// MaxLeadOriginDetailRunes is the bound on the note beside a lead's origin. The
// database holds the same number in the lead_origin_detail_bounded check, and the
// contacts store and the capture sink both cut to it before they write.
const MaxLeadOriginDetailRunes = 500
