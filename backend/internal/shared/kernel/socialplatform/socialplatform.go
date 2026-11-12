// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package socialplatform names the keys a contact's social links may hold. The
// contacts module refuses any other key, and record_fields lists these, so the
// list lives below both of them.
package socialplatform

import "slices"

// The platforms a contact's social map takes.
const (
	GitHub   = "github"
	LinkedIn = "linkedin"
	Twitter  = "twitter"
)

// LinkedInProfileExample is the one shape the linkedin key accepts.
const LinkedInProfileExample = "https://www.linkedin.com/in/<name>"

// Keys answers every platform, in the order a reader is shown them.
func Keys() []string { return []string{GitHub, LinkedIn, Twitter} }

// Known reports whether key is a platform a contact's social map takes.
func Known(key string) bool { return slices.Contains(Keys(), key) }
