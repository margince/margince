// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/socialplatform"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ContactSocialInput preserves social identity inputs across creation and preview.
type ContactSocialInput struct {
	Platform string
	Handle   string
}

// contactSocialInputs lists the handles a social map carries, in platform
// order. A key outside socialplatform.Keys is refused by name. A key sent empty
// is a removal and carries no handle. The linkedin key takes only a profile
// link, stored in the normalizer's spelling, because the exact-match dedupe
// compares it.
func contactSocialInputs(social map[string]any) ([]ContactSocialInput, error) {
	keys := make([]string, 0, len(social))
	for platform := range social {
		keys = append(keys, platform)
	}
	sort.Strings(keys)
	out := make([]ContactSocialInput, 0, len(keys))
	for _, platform := range keys {
		if !socialplatform.Known(platform) {
			return nil, &values.ParseError{
				Field: "social." + platform, Code: "social_platform_unknown",
				Message: "a contact's social links take only " + strings.Join(socialplatform.Keys(), ", "),
			}
		}
		handle := socialHandle(social[platform])
		if strings.TrimSpace(handle) == "" {
			continue
		}
		if platform == SocialLinkedIn {
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

// socialHandle is the text a social value carries. A JSON null is a removal,
// so it reads as empty rather than as the word "<nil>".
//
//craft:ignore naked-any a social map is the contract's open object, so a value arrives untyped
func socialHandle(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// socialKeysThatMoved answers the social keys whose handle differs between two
// readings, each with its handle on that side; an absent key reads as null.
//
//craft:ignore naked-any the images are the contract's open social object
func socialKeysThatMoved(stored, written *map[string]any) (was, now map[string]any) {
	var before, after map[string]any
	if stored != nil {
		before = *stored
	}
	if written != nil {
		after = *written
	}
	was, now = map[string]any{}, map[string]any{}
	for _, side := range []map[string]any{before, after} {
		for key := range side {
			if before[key] != after[key] {
				was[key], now[key] = before[key], after[key]
			}
		}
	}
	return was, now
}

// socialWithLinkedInURL folds a request's top-level linkedin_url into its
// social map, so both spellings write the one contact_social row. cleared says
// the request sent linkedin_url as null, which removes the link. A request
// naming two different profiles under the two spellings is refused.
//
//craft:ignore naked-any a social map is the contract's open object
func socialWithLinkedInURL(social map[string]any, url *string, cleared bool) (map[string]any, error) {
	if url == nil && !cleared {
		return social, nil
	}
	link := ""
	if url != nil {
		link = *url
	}
	if sent, named := social[SocialLinkedIn]; named && !sameProfile(socialHandle(sent), link) {
		return nil, &values.ParseError{
			Field: linkedinURLField, Code: "linkedin_url_conflicts_with_social",
			Message: "linkedin_url and social.linkedin name two different profiles; send one of them",
		}
	}
	folded := maps.Clone(social)
	if folded == nil {
		folded = map[string]any{}
	}
	folded[SocialLinkedIn] = link
	return folded, nil
}

// sameProfile reports whether two LinkedIn values name one profile: equal in
// the stored spelling when both parse, equal as typed otherwise.
func sameProfile(a, b string) bool {
	normalA, errA := NormalizeContactLinkedInURL(a)
	normalB, errB := NormalizeContactLinkedInURL(b)
	if errA == nil && errB == nil {
		return normalA == normalB
	}
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}
