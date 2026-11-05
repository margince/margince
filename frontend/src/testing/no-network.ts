// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The runtime's own fetch, captured before vitest.setup.ts puts a guard over
// `globalThis.fetch`.
//
// A case that stands up its own server stubs this back in. That reads as a
// claim about the case, not as a hole in the guard.
export const fetchTheNetwork = globalThis.fetch;
