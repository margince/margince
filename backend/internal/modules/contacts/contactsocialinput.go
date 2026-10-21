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

func contactSocialInputs(social map[string]any) []ContactSocialInput {
	keys := make([]string, 0, len(social))
	for platform := range social {
		keys = append(keys, platform)
	}
	sort.Strings(keys)
	out := make([]ContactSocialInput, 0, len(keys))
	for _, platform := range keys {
		handle := fmt.Sprintf("%v", social[platform])
		if strings.TrimSpace(platform) != "" && strings.TrimSpace(handle) != "" {
			out = append(out, ContactSocialInput{Platform: platform, Handle: handle})
		}
	}
	return out
}
