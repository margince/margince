// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useEffectEvent, useState } from "react";
import { routeIdentity } from "./routeidentity";
import { parseHash, useHash, useLiveHash } from "./router";

const pageOf = (hash: string) => routeIdentity(parseHash(hash));

type Overlay = Readonly<{ open: boolean; close: () => void; held?: boolean }>;

/**
 * Closes an open overlay when the page under it changes; a dial written in
 * place keeps the page.
 */
export function useClosesWithPage({ open, close, held = false }: Overlay) {
  const live = pageOf(useLiveHash());
  // A screen on its way out, or held by the unsaved question, reads its own
  // address here: its overlays leave or stay with it.
  const drawnOn = pageOf(useHash());
  const [page, setPage] = useState({ on: live, leaves: 0 });
  if (page.on !== live && (!open || (drawnOn === live && !held))) {
    setPage({ on: live, leaves: open ? page.leaves + 1 : page.leaves });
  }
  const leave = useEffectEvent(close);
  useEffect(() => {
    if (page.leaves > 0) {
      leave();
    }
  }, [page.leaves]);
}

/**
 * The hook, for an overlay drawn only while it shows: a closed one reads no
 * address.
 */
export function ClosesWithPage(overlay: Overlay): null {
  useClosesWithPage(overlay);
  return null;
}
