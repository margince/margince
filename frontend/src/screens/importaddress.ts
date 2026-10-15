// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { routeHash } from "../lib/router";
import {
  currentParams,
  hashWithParams,
  replaceParams,
  useUrlParams,
} from "../lib/urlstate";
import { IMPORT_RUN_SUBPAGE, settingsHref } from "./settingsrouting";

const RUN_PARAM = "run";

/**
 * The run the page is about, held in the address (`?run=`). A reader who
 * leaves a running import and comes back, from Recent imports or by reload,
 * lands on the same run. The server answers 404 for a run that is not theirs.
 */
export function useAddressedRun(): [
  string | null,
  (id: string | null) => void,
] {
  const [params] = useUrlParams();
  const id = params.get(RUN_PARAM) || null;
  const address = (next: string | null) => {
    const dials = new Map(currentParams());
    if (next) {
      dials.set(RUN_PARAM, next);
    } else {
      dials.delete(RUN_PARAM);
    }
    replaceParams(dials);
  };
  return [id, address];
}

/** The address of the run page opened on one run. */
export function runHref(id: string): string {
  return hashWithParams(
    routeHash({ ...settingsHref("import"), id2: IMPORT_RUN_SUBPAGE }),
    new Map([[RUN_PARAM, id]]),
  );
}
