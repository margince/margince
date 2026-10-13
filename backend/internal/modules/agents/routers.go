// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// RegisterRouters registers the routers whose targets come from several
// Register* calls, so it runs after all of them. A router whose targets share
// one seam registers beside them, and is absent when that seam is.
func RegisterRouters(r *Registry) {
	r.Register(changeLeadState{})
	r.Register(describeVocabulary{})
	r.Register(readInputChecks{})
}
