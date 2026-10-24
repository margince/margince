// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import {
  jsonResponse,
  type RouteMap,
  WithInstallationBrand,
} from "../testing/story-utils";
import { CommandPalette, useBuiltinCommands } from "./palette";
import { Shell } from "./shell";

// The shell's frames draw on these, and a full-page story mounts the same frame
// around a screen, so both reach for one copy.

// One queue, read twice: the panel's Decisions badge and the strip's chip.
export const COUNTS = { inbox: 12, tasks: 4 };

// The routes the chrome itself reads on every page, under whatever the page asks.
export const SHELL_ROUTES: RouteMap = {
  "GET /ai/usage": () =>
    jsonResponse({
      days: [],
      budget: { monthly_tokens: 100, spent_tokens: 20, band: "normal" },
    }),
};

// No logo_url: Storybook serves no object store, so the monogram is what a
// company without a declared icon gets, and the case worth framing.
export function SeedInstallation({
  children,
  logoUrl,
  logoIconUrl,
}: Readonly<{
  children: ReactNode;
  logoUrl?: string;
  logoIconUrl?: string;
}>) {
  return (
    <WithInstallationBrand
      brand={{
        display_name: "Gradion GmbH",
        logo_url: logoUrl,
        logo_icon_url: logoIconUrl,
      }}
    >
      {children}
    </WithInstallationBrand>
  );
}

// The top bar's field is the only pointer route into the palette, so the frame
// mounts the palette beside the shell the way App.tsx does.
export function usePaletteSeam() {
  const [open, setOpen] = useState(false);
  const commands = useBuiltinCommands();
  const palette = (
    <CommandPalette
      open={open}
      onClose={() => setOpen(false)}
      commands={commands}
    />
  );
  return { openSearch: () => setOpen(true), palette };
}

export function ShellExample({
  at = "#/",
  children,
}: Readonly<{
  // Stated per frame: the catalog keeps one iframe across stories, so an unset
  // address would be whatever the previous frame visited.
  at?: string;
  children: ReactNode;
}>) {
  // Set once on open, not on every render: the rail's rows are real links, and
  // restating the address would undo the reader's own navigation.
  useState(() => {
    globalThis.location.hash = at;
  });
  const { openSearch, palette } = usePaletteSeam();
  return (
    <>
      <Shell onOpenSearch={openSearch} counts={COUNTS}>
        {children}
      </Shell>
      {palette}
    </>
  );
}
