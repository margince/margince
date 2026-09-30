// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// localOnlyAdmits reports whether a local_only task may be served over a
// binding whose endpoint sits at isLocal. Both the ladder (servableLadder)
// and the decision lane (decisionSkipFor) read this one function, so
// restoring the local-only guarantee is a single edit both call sites
// inherit — rather than two enforcement points that can drift the way #6244
// found the ladder's and the decision lane's had.
//
// Currently unconditional: #6396 reverted the ladder's own narrowing to
// same-host rungs because local_small is not reliably local across shipped
// presets, and the premise is not settled (#3351). The decision lane follows
// the same reverted premise rather than holding a guarantee the ladder no
// longer does.
func localOnlyAdmits(Task, bool) bool { return true }
