// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { navigate, routeHash } from "../../lib/router";
import { IMPORT_RUN_SUBPAGE, settingsHref } from "../../lib/settingsrouting";
import {
  currentParams,
  hashWithParams,
  replaceParams,
  useUrlParams,
} from "../../lib/urlstate";

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

const OBJECT_PARAM = "object";
const CONTACT_PARAM = "contact";
const ACTIVITY_OBJECT = "activity";

/**
 * The contact an import started from a contact's page files its activities
 * under, held in the address (`?object=activity&contact=`). Null for an
 * ordinary import, which names no record up front.
 */
export function usePresetContact(): string | null {
  const [params] = useUrlParams();
  if (params.get(OBJECT_PARAM) !== ACTIVITY_OBJECT) return null;
  return params.get(CONTACT_PARAM) || null;
}

/** Opens the run page for a file of activities that all belong to one contact. */
export function openActivityImport(contactId: string): void {
  navigate(
    { ...settingsHref("import"), id2: IMPORT_RUN_SUBPAGE },
    new Map([
      [OBJECT_PARAM, ACTIVITY_OBJECT],
      [CONTACT_PARAM, contactId],
    ]),
  );
}
