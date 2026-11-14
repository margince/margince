// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { CF_OBJECTS, type CfObject } from "../lib/customfields.logic";
import type { Screen } from "../lib/router";

// Past this a catalog read that has not answered stops holding the dialog.
export const CATALOG_WAIT_MS = 1_500;

// The catalog a create from each list screen merges into its form.
const CATALOG_OF_SCREEN: Partial<Record<Screen, CfObject>> = {
  contacts: "contact",
  companies: "company",
  deals: "deal",
  leads: "lead",
  projects: "project",
};

export function catalogOfScreen(screen: Screen): CfObject | undefined {
  return CATALOG_OF_SCREEN[screen];
}

// An edit's record key is the catalog's own object name where it has one.
export function catalogOfRecord(recordKey: string): CfObject | undefined {
  return CF_OBJECTS.find((object) => object === recordKey);
}

// Not while `catalog`'s first read, retries included, is in flight: the form
// would open half-built. A read that fails for good or is slow opens the core.
export function useSettledOpen(open: boolean, catalog?: CfObject): boolean {
  const client = useQueryClient();
  const catalogHash = client.defaultQueryOptions({
    queryKey: ["custom-fields", catalog],
  }).queryHash;
  const cache = client.getQueryCache();
  const subscribe = useCallback(
    (changed: () => void) => {
      if (!open || catalog === undefined) return () => undefined;
      return cache.subscribe((event) => {
        if (event.query.queryHash === catalogHash) changed();
      });
    },
    [cache, catalogHash, catalog, open],
  );
  const snapshot = useCallback(() => {
    if (!open || catalog === undefined) return false;
    const state = cache.get(catalogHash)?.state;
    return state?.fetchStatus === "fetching" && state.data === undefined;
  }, [cache, catalogHash, catalog, open]);
  const reading = useSyncExternalStore(subscribe, snapshot, () => false);
  const [waited, setWaited] = useState(false);
  useEffect(() => {
    if (!open) {
      setWaited(false);
      return;
    }
    const timer = setTimeout(() => setWaited(true), CATALOG_WAIT_MS);
    return () => clearTimeout(timer);
  }, [open]);
  const [shown, setShown] = useState(false);
  const next = open && (shown || waited || !reading);
  if (next !== shown) {
    setShown(next);
  }
  return next;
}
