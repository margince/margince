// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { TrialNotice } from "../components/trialcontact";
import { AppLink } from "../design-system/applink";
import { Callout } from "../design-system/callout";
import { calendarDay } from "../format/calendarday";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import {
  type TrialStanding,
  useLicenseEntitlement,
  useLicensePosture,
  useTrialStanding,
} from "../lib/license-posture";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import { useViewerZone } from "../lib/viewerzone";

const COPY = {
  refused: { title: "shell.license.refused", body: "licensebanner.body" },
  grace: {
    title: "licensebanner.grace.title",
    body: "licensebanner.grace.body",
  },
} as const;

// A refused or expired licence is a standing condition, not agent work. So it
// stands in the shell's chrome and not on the rail's live line. Not `none`:
// every dev and demo stack has no licence, and a notice on all of them is noise.
//
// The trial's days live on the orb and its pill. The chrome joins in only for
// the last week. A reader may put it away until the next day, but not on the
// last one.
export function LicenseBanner() {
  const t = useT();
  const notice = useLicenseNotice();
  const entitlement = useLicenseEntitlement(notice?.kind === "trial");
  const zone = useViewerZone();
  const today = calendarDay(new Date(useNow(60_000)), zone);
  const [hiddenOn, setHiddenOn] = useState(() =>
    readStored(STORAGE_KEYS.trialNoticeHiddenOn),
  );
  if (notice === null) {
    return null;
  }
  if (notice.kind === "trial") {
    const { trial } = notice;
    const lastDay = trial.ended || trial.daysLeft <= 1;
    if (!lastDay && hiddenOn === today) {
      return null;
    }
    const hide = () => {
      writeStored(STORAGE_KEYS.trialNoticeHiddenOn, today);
      setHiddenOn(today);
    };
    return (
      <div className="appbanner">
        <TrialNotice
          standing={trial}
          seatsUsed={entitlement.data?.seats_used}
          onHide={lastDay ? undefined : hide}
        />
      </div>
    );
  }
  const copy = COPY[notice.kind];
  return (
    // `warning` rather than `danger`, and no dismiss: escalating would make the
    // chrome a sales surface, and the repair behind it stays until it is made.
    <div className="appbanner">
      <Callout
        tone="warning"
        kind="standing"
        title={t(copy.title)}
        actions={
          <AppLink href={routeHash(settingsHref("seats"))}>
            {t("licensebanner.link", { tab: t("settings.tab.seats") })}
          </AppLink>
        }
      >
        {t(copy.body)}
      </Callout>
    </div>
  );
}

type LicenseNotice =
  | Readonly<{ kind: "trial"; trial: TrialStanding }>
  | Readonly<{ kind: keyof typeof COPY }>;

/** What the license asks the chrome to say, before a reader hides it for the day. */
export function useLicenseNotice(): LicenseNotice | null {
  const posture = useLicensePosture();
  const trial = useTrialStanding();
  if (trial) {
    return trial.closing ? { kind: "trial", trial } : null;
  }
  return posture === "refused" || posture === "grace"
    ? { kind: posture }
    : null;
}
