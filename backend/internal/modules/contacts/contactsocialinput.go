// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"sort"
	"strings"
)

// ContactSocialInput preserves social identity inputs across creation and preview.
type ContactSocialInput struct {
	Platform string
	Handle   string
}

// contactSocialInputs lists the handles a social map carries, in platform
// order. A LinkedIn URL is stored in the normalizer's spelling. A company page
// there is refused, because the slot is a profile and the exact-match
// dedupe compares it.
func contactSocialInputs(social map[string]any) ([]ContactSocialInput, error) {
	keys := make([]string, 0, len(social))
	for platform := range social {
		keys = append(keys, platform)
	}
	sort.Strings(keys)
	out := make([]ContactSocialInput, 0, len(keys))
	for _, platform := range keys {
		handle := fmt.Sprintf("%v", social[platform])
		if strings.TrimSpace(platform) == "" || strings.TrimSpace(handle) == "" {
			continue
		}
		if platform == SocialLinkedIn && isLinkedinURL(handle) {
			normalized, err := NormalizeContactLinkedInURL(handle)
			if err != nil {
				return nil, err
			}
			handle = normalized
		}
		out = append(out, ContactSocialInput{Platform: platform, Handle: handle})
	}
	return out, nil
}
