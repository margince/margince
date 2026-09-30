// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CloudOff, type LucideIcon, WifiOff } from "lucide-react";
import { useState } from "react";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type Connectivity,
  type Outage,
  useConnectivity,
} from "./connectivity";

const NOTICE: Readonly<
  Record<Outage, { icon: LucideIcon; title: MessageKey; body: MessageKey }>
> = {
  offline: {
    icon: WifiOff,
    title: "connectivity.offline.title",
    body: "connectivity.offline.body",
  },
  unreachable: {
    icon: CloudOff,
    title: "connectivity.unreachable.title",
    body: "connectivity.unreachable.body",
  },
};

export function ConnectivityBanner() {
  return <ConnectivityNotice state={useConnectivity()} />;
}

/** The condition while it holds, and no dismiss: it is a state, and it goes
 *  when the connection comes back. */
export function ConnectivityNotice({
  state,
}: Readonly<{ state: Connectivity }>) {
  const t = useT();
  const [shown, setShown] = useState(state);
  const [restored, setRestored] = useState(false);
  if (state !== shown) {
    setShown(state);
    setRestored(state === "online");
  }
  const notice = state === "online" ? null : NOTICE[state];
  return (
    // Mounted before its words arrive, since a region inserted with them is not
    // reliably read; the notice inside is `standing`, so it is heard only once.
    <div
      className={notice === null ? undefined : "appbanner"}
      aria-live="polite"
      aria-atomic="true"
    >
      {notice !== null && (
        // `warning` and not `danger`: nothing is lost yet, and the write that
        // does fail says so in its own danger line.
        <Callout
          tone="warning"
          kind="standing"
          icon={notice.icon}
          title={t(notice.title)}
        >
          {t(notice.body)}
        </Callout>
      )}
      {notice === null && restored && (
        <span className="sr-only">{t("connectivity.restored")}</span>
      )}
    </div>
  );
}
