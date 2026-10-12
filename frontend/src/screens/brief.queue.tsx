// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useId, useRef } from "react";
import { navigateReplacing, useHash } from "../app/router";
import { useScrollMemory } from "../app/scrollmemory";
import { useUrlParams } from "../app/urlstate";
import { Modal } from "../design-system/atoms";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { WorklistScreen } from "./worklist";
import { drawerScope } from "./worklist.address";
import { worklistFilterFrom } from "./worklist.header";
import { WorklistReturnScope } from "./worklist.return";
import "./brief.css";

/** Older shared links open the same queue, including its owner and narrowing. */
export function WorklistRedirect({ opensOn }: Readonly<{ opensOn?: string }>) {
  const [params] = useUrlParams();
  useEffect(() => {
    const next = new Map(params);
    next.set("queue", "1");
    next.delete("scope");
    if (params.get("scope"))
      next.set("queue_scope", params.get("scope") ?? "mine");
    else if (opensOn === "unassigned") next.set("queue_scope", "unassigned");
    if (opensOn && opensOn !== "unassigned") next.set("owner", opensOn);
    navigateReplacing({ screen: "home" }, next);
  }, [opensOn, params]);
  return null;
}

export function BriefQueue() {
  const t = useT();
  const titleId = useId();
  const body = useRef<HTMLDivElement>(null);
  const [params, setParams] = useUrlParams();
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
  const close = () => {
    const next = new Map(params);
    next.delete("queue");
    setParams(next);
  };
  return (
    <Modal
      open={params.get("queue") === "1"}
      onClose={close}
      labelledBy={titleId}
      intent="drawer-reading"
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
          <WorklistScreen opensOn={owner} embedded />
        </WorklistReturnScope>
      </DrawerBody>
    </Modal>
  );
}
