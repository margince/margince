// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// RegisterRouters registers every router. It runs after every other Register*
// call, because a router's spec is derived from targets that must already be
// registered.
func RegisterRouters(r *Registry) {
	r.Register(changeLeadState{})
	r.Register(describeVocabulary{})
}
