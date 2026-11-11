// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../design-system/applink";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { useLicensePosture } from "../lib/license-posture";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

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
export function LicenseBanner() {
  const t = useT();
  const posture = useLicensePosture();
  if (posture !== "refused" && posture !== "grace") {
    return null;
  }
  const copy = COPY[posture];
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
