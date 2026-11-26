// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// LinkedInProfileKeySQL is the profile a LinkedIn link names, in SQL: its slug
// under /in/, lower case, or NULL when the link names no profile. A localized
// host, a trailing slash, a query or a deeper path name the same profile.
// idx_contact_social_linkedin_profile indexes this expression over
// contact_social.handle, so a reader that spells it any other way loses the
// index. The contacts module's linkedInProfileKey is its Go twin.
func LinkedInProfileKeySQL(expr string) string {
	return SQLf(`substring(lower(%s) from '^(?:https?://)?(?:[a-z0-9-]+\.)*linkedin\.com/in/([^/?#]+)')`, expr)
}
