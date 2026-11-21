// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { parseHash } from "./router";

// Every address boots on the essential catalog. These screens draw from it alone:
// their code is an initial entry (scripts/vite-entry-chunks.ts), whose copy it holds.
const ESSENTIAL_SCREENS: ReadonlySet<string> = new Set(["analytics", "home"]);

export function paintsFromEssentialCopy(hash: string): boolean {
  return ESSENTIAL_SCREENS.has(parseHash(hash).screen);
}
