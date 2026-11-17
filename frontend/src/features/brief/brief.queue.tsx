// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ComponentProps,
  type ReactNode,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import type { DrawerSpan } from "../../design-system/modal";
import { useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";
import { closeOnto } from "../../lib/navigatefrom";
import { navigate, useHash } from "../../lib/router";
import { useScrollMemory } from "../../lib/scrollmemory";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";
import {
  currentParams,
  NO_URL_PARAMS,
  type UrlParams,
  useAddressWriters,
  useUrlParams,
} from "../../lib/urlstate";
import { WORKLIST_VIEW_PARAM } from "../../lib/worklistrouting";
import { drawerScope } from "../worklist/worklist.address";
import { worklistFilterFrom } from "../worklist/worklist.header";
import { WorklistReturnScope } from "../worklist/worklist.return";
import { Pending } from "./brief.deferred";
import "./brief.css";

/** The Worklist screen the drawer holds, loaded when the drawer opens. */
export const loadWorklistScreen = () => import("../worklist/worklist");

const WorklistScreen = lazy<
  NonNullable<
    ComponentProps<typeof import("../worklist/worklist").WorklistScreen>
  >
>(
  async () => ({ default: (await loadWorklistScreen()).WorklistScreen }),
  "complete",
  Pending,
);

// The dials that narrow the drawer. Opening from Home clears them, so it always
// shows the reader's own Today with every category; closing drops them too.
const DRAWER_DIALS = [
  "queue_scope",
  WORKLIST_VIEW_PARAM,
  "filter",
  "owner",
  "selected",
];

/** Whether these dials open Home's queue drawer. */
export const drawerOpen = (params: UrlParams): boolean =>
  params.get("queue") === "1";

/** Close the drawer onto Home as it stood under it. */
export function closeQueue(): void {
  const next = new Map(currentParams());
  if (!drawerOpen(next)) return;
  next.delete("queue");
  for (const dial of DRAWER_DIALS) next.delete(dial);
  closeOnto({ screen: "home" }, next);
}

/**
 * Open the drawer on a new entry, so Back closes it. It reads the address as
 * it is now: a second press before the first has drawn adds no second entry.
 */
export function openQueue(): void {
  const next = new Map(currentParams());
  if (drawerOpen(next)) return;
  for (const dial of DRAWER_DIALS) next.delete(dial);
  next.set("queue", "1");
  navigate({ screen: "home" }, next);
}

/** Older shared links open the same queue, including its owner and narrowing. */
export function WorklistRedirect({ opensOn }: Readonly<{ opensOn?: string }>) {
  const [params] = useUrlParams();
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    const next = new Map(params);
    next.set("queue", "1");
    next.delete("scope");
    if (params.get("scope"))
      next.set("queue_scope", params.get("scope") ?? "mine");
    else if (opensOn === "unassigned") next.set("queue_scope", "unassigned");
    if (opensOn && opensOn !== "unassigned") next.set("owner", opensOn);
    navigateReplacing({ screen: "home" }, next);
  }, [opensOn, params, navigateReplacing]);
  return null;
}

/** Home's list is the Worklist at default dials, so records opened from it keep the way back. */
export function HomeListReturnScope({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <WorklistReturnScope dials={NO_URL_PARAMS}>{children}</WorklistReturnScope>
  );
}

export function BriefQueue() {
  const t = useT();
  const titleId = useId();
  const body = useRef<HTMLDivElement>(null);
  const [params] = useUrlParams();
  // Kept per narrowing for the whole load rather than per history entry: a
  // record's "Back to Worklist" reopens the drawer on a NEW entry, and the
  // reader expects the place they left in this same list.
  const owner = params.get("owner");
  useScrollMemory(
    body,
    useHash(),
    `worklist-drawer:${drawerScope(params)}:${worklistFilterFrom(params)}:${owner ?? ""}`,
    "load",
  );
  // Kept by this browser, so every way into the drawer opens it as wide as the
  // reader left it. A shared link carries no one's layout.
  const [span, setSpan] = useState<DrawerSpan>(() =>
    readStored(STORAGE_KEYS.worklistDrawerWide) === "1" ? "window" : "intent",
  );
  const changeSpan = (next: DrawerSpan) => {
    setSpan(next);
    writeStored(STORAGE_KEYS.worklistDrawerWide, next === "window" ? "1" : "0");
  };
  return (
    <Modal
      open={drawerOpen(params)}
      onClose={closeQueue}
      labelledBy={titleId}
      intent="drawer-reading"
      span={span}
      onSpanChange={changeSpan}
    >
      {/* The BANDED head, not the composer's one scrolling column: the queue is
          a list a reader pages through, so its title and the dialog's own close
          have to stay put while the rows move under them. Everything about how
          the band is spaced is the shared drawer's (atoms.css) — this head adds
          nothing, because a queue title is not a different kind of title. */}
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("brief.queue.title")}
        </Heading>
      </DrawerHead>
      <DrawerBody className="brief-queue-body" ref={body}>
        <WorklistReturnScope>
          <WorklistScreen embedded />
        </WorklistReturnScope>
      </DrawerBody>
    </Modal>
  );
}
