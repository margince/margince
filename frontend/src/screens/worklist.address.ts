// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useUrlParams } from "../app/urlstate";
import { worklistFilterFrom } from "./worklist.header";
import {
  UNASSIGNED,
  type WorklistFilter,
  type WorklistScope,
} from "./worklist.queries";

/**
 * Which queue the DRAWER is on, off the address it keeps its state in.
 *
 * Exported because the Brief's own head reads it too: the way into the queue
 * carries the queue's total, and the drawer's scope survives the drawer
 * closing — so a count taken from the reader's personal day sat on a button
 * that opens the team's. One spelling, so the button and the list it opens
 * cannot name two different queues.
 */
export function drawerScope(
  params: ReadonlyMap<string, string>,
): WorklistScope {
  return requestedScope(params.get("queue_scope"));
}

function requestedScope(
  value: string | undefined,
  opensOn?: string,
): WorklistScope {
  if (
    value === "mine" ||
    value === "team" ||
    value === "all" ||
    value === UNASSIGNED
  )
    return value;
  return opensOn === UNASSIGNED ? UNASSIGNED : "mine";
}

/** Embedded queue state survives closing the drawer and follows shared links. */
export function useWorklistAddress(
  opensOn: string | undefined,
  embedded: boolean,
) {
  const [params, setParams] = useUrlParams();
  const [localOwner, setLocalOwner] = useState(
    opensOn && opensOn !== UNASSIGNED ? opensOn : "",
  );
  const [localSelected, setLocalSelected] = useState("");
  const scopeParam = embedded ? "queue_scope" : "scope";
  const scope = requestedScope(params.get(scopeParam), opensOn);
  const filter = worklistFilterFrom(params);
  const owner = embedded ? (params.get("owner") ?? "") : localOwner;
  const selectedId = embedded ? (params.get("selected") ?? "") : localSelected;
  const change = (key: string, value: string) => {
    const next = new Map(params);
    if (value) next.set(key, value);
    else next.delete(key);
    if (key !== "selected") {
      next.delete("selected");
      setLocalSelected("");
    }
    setParams(next);
  };
  const setScope = (next: WorklistScope) => change(scopeParam, next);
  const setFilter = (next: WorklistFilter) =>
    change("filter", next === "all" ? "" : next);
  const setOwner = (next: string) => {
    if (embedded) change("owner", next);
    else {
      setLocalSelected("");
      setLocalOwner(next);
    }
  };
  const setSelectedId = (next: string) => {
    if (embedded) change("selected", next);
    else setLocalSelected(next);
  };
  return {
    scope,
    filter,
    owner,
    selectedId,
    setScope,
    setFilter,
    setOwner,
    setSelectedId,
  };
}
