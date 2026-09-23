// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { settingsHref } from "../screens/settingsrouting";
import { useLicensePosture } from "./agentrail-reads";
import { routeHash } from "./router";

// A refused licence is a standing installation condition, not something the
// agent is doing, so it stands in the shell's chrome rather than on the rail's
// live line. Only `refused`: every dev and demo stack has no licence at all,
// and a notice drawn on all of them would stop being a signal.
export function LicenseBanner() {
  const t = useT();
  const posture = useLicensePosture();
  if (posture !== "refused") {
    return null;
  }
  return (
    // `warning` rather than `danger`, and no dismiss: escalating would make the
    // chrome a sales surface, and the repair behind it stays until it is made.
    <div className="appbanner">
      <Callout
        tone="warning"
        kind="standing"
        title={t("shell.license.refused")}
        actions={
          <a href={routeHash(settingsHref("seats"))}>
            {t("licensebanner.link", { tab: t("settings.tab.seats") })}
          </a>
        }
      >
        {t("licensebanner.body")}
      </Callout>
    </div>
  );
}
