// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useEffectEvent, useState } from "react";
import { routeIdentity } from "./routeidentity";
import { parseHash, useHash, useLiveHash } from "./router";

const screenIdentityOf = (hash: string) => routeIdentity(parseHash(hash));

type Overlay = Readonly<{ open: boolean; close: () => void; held?: boolean }>;

/**
 * Closes an open overlay when the screen under it changes; a dial or a tab of
 * the same record keeps it.
 */
export function useClosesWithPage({ open, close, held = false }: Overlay) {
  const live = screenIdentityOf(useLiveHash());
  // A screen on its way out, or held by the unsaved question, reads its own
  // address here: its overlays leave or stay with it.
  const drawnOn = screenIdentityOf(useHash());
  const [under, setUnder] = useState({ on: live, leaves: 0 });
  if (under.on !== live && (!open || (drawnOn === live && !held))) {
    setUnder({ on: live, leaves: open ? under.leaves + 1 : under.leaves });
  }
  const leave = useEffectEvent(close);
  useEffect(() => {
    if (under.leaves > 0) {
      leave();
    }
  }, [under.leaves]);
}

/**
 * The hook, for an overlay drawn only while it shows: a closed one reads no
 * address.
 */
export function ClosesWithPage(overlay: Overlay): null {
  useClosesWithPage(overlay);
  return null;
}
