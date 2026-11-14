// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { parseHash } from "./router";

// The screens a cold load paints from the essential catalog. Their own code is
// an initial entry (scripts/vite-entry-chunks.ts), so the essential catalog
// holds every message they draw; any other screen waits for the complete one.
const ESSENTIAL_SCREENS: ReadonlySet<string> = new Set(["analytics", "home"]);

export function bootsOnEssentialCopy(hash: string): boolean {
  return ESSENTIAL_SCREENS.has(parseHash(hash).screen);
}
