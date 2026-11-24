// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// scripts/vite-screen-chunks.ts writes this block into the shell, and
// App.warmchunks.ts reads it.
export const SCREEN_CHUNKS_ID = "screen-chunks";

/** Every file once, and each screen's scripts and stylesheets by index. */
export type ScreenChunks = {
  files: string[];
  screens: Record<string, number[]>;
};
