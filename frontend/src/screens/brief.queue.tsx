// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useId, useRef } from "react";
import { navigateReplacing } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import { Button, Modal } from "../design-system/atoms";
import { useT } from "../i18n";
import { WorklistScreen } from "./worklist";
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

// `closeLabel` names where closing LEAVES the reader: "Back to Home" on the
// Home page the drawer was written for, and the plain word on a record page
// that mounts the same drawer over itself.
export function BriefQueue({
  closeLabel,
}: Readonly<{ closeLabel?: string }> = {}) {
  const t = useT();
  const titleId = useId();
  const scroll = useRef(0);
  const [params, setParams] = useUrlParams();
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
      placement="right"
      size="split"
    >
      <div className="drawer-head brief-queue-head">
        <h2 id={titleId} className="t-h2">
          {t("brief.queue.title")}
        </h2>
        <Button variant="ghost" onClick={close}>
          {closeLabel ?? t("brief.queue.close")}
        </Button>
      </div>
      <div
        className="drawer-body brief-queue-body"
        ref={(element) => {
          if (element) element.scrollTop = scroll.current;
        }}
        onScroll={(event) => {
          scroll.current = event.currentTarget.scrollTop;
        }}
      >
        <WorklistScreen opensOn={params.get("owner")} embedded />
      </div>
    </Modal>
  );
}
