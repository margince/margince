// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useIsFetching } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import type { Screen } from "../app/router";
import { CF_OBJECTS, type CfObject } from "./customfields.logic";

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
  const reading = useIsFetching({
    queryKey: ["custom-fields", catalog],
    exact: true,
    predicate: (query) =>
      catalog !== undefined && query.state.data === undefined,
  });
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
  const next = open && (shown || waited || reading === 0);
  if (next !== shown) {
    setShown(next);
  }
  return next;
}
