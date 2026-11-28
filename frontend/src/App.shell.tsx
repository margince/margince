// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useWarmRouteChunks } from "./App.warmchunks";
import { AuthSplash } from "./app/bootsplash";
import { RaillessFrame } from "./app/railless";
import { useRecordPrefetch } from "./app/recordprefetch";
import { Shell } from "./app/shell";
import { useWarmProgressName } from "./lib/progressreader";

export type OnboardingHold = Readonly<{ held: boolean; turnedAway: boolean }>;

// A unit's screen in an allowed frame gets no shell. The frame's host draws its
// own chrome, and the shell's reads and navigation belong to the app.
export function ScreenFrame({
  embedded,
  children,
  hold,
  onOpenSearch,
}: Readonly<{
  embedded: boolean;
  children: ReactNode;
  hold: OnboardingHold;
  onOpenSearch: () => void;
}>) {
  if (embedded) {
    return <RaillessFrame>{children}</RaillessFrame>;
  }
  return (
    <AuthedShell hold={hold} onOpenSearch={onOpenSearch}>
      {children}
    </AuthedShell>
  );
}

function AuthedShell({
  children,
  hold,
  onOpenSearch,
}: Readonly<{
  children: ReactNode;
  hold: OnboardingHold;
  onOpenSearch: () => void;
}>) {
  useWarmRouteChunks();
  useRecordPrefetch();
  useWarmProgressName(!hold.held);
  // The gate is replacing this address, and its shell would only start the
  // full app's reads.
  if (hold.turnedAway) {
    return (
      <RaillessFrame>
        <AuthSplash />
      </RaillessFrame>
    );
  }
  return (
    <Shell onOpenSearch={onOpenSearch} held={hold.held}>
      {children}
    </Shell>
  );
}
