// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SetupSteps } from "../../components/setup-steps";
import { AppLink } from "../../design-system/applink";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanMutate } from "../../lib/capability";
import { grantStanding, useAgentGrants } from "../../lib/overnight-grant";
import { routeHash } from "../../lib/router";
import { useSettingsReach } from "../../lib/settingsreach";
import { settingsHref } from "../../lib/settingsrouting";
import { useSetupChecklist } from "../../lib/setup-checklist";

// Home before the reader's first Morning brief: what the page is for, when the
// brief comes, and what to set up first. Drawn only while no run exists.

/** What the first brief waits on, as far as this reader can change it. */
export type FirstBriefWait = "prepared" | "switchedOff" | "notTheirs";

const WAIT_LINE: Readonly<Record<FirstBriefWait, MessageKey>> = {
  prepared: "brief.firstDay.briefTonight",
  switchedOff: "brief.firstDay.briefOff",
  notTheirs: "brief.firstDay.briefLater",
};

/**
 * Which sentence the first brief gets. A read seat writes nothing, so it is
 * never sent to a switch it cannot turn; an unread answer claims nothing.
 */
export function firstBriefWait(
  canMutate: boolean,
  granted: boolean | undefined,
): FirstBriefWait {
  if (!canMutate || granted === undefined) {
    return "notTheirs";
  }
  return granted ? "prepared" : "switchedOff";
}

// A lapsed or outgrown grant prepares nothing tonight, so it reads as off and
// the link leads to the card that renews it.
function grantWorks(
  grants: Parameters<typeof grantStanding>[0] | undefined,
): boolean | undefined {
  if (grants === undefined) return undefined;
  const { granted, lapsed, outgrown } = grantStanding(grants);
  return granted && !lapsed && !outgrown;
}

export function FirstDayPanel() {
  const t = useT();
  const canMutate = useCanMutate();
  const grants = useAgentGrants(canMutate);
  const setup = useSetupChecklist(useSettingsReach());
  const wait = firstBriefWait(canMutate, grantWorks(grants.data));
  const openSteps = setup.settled && setup.total > 0 && !setup.complete;
  return (
    <Panel title={t("brief.firstDay.title")}>
      <PanelBody>
        <PanelIntro>{t("brief.firstDay.what")}</PanelIntro>
        <p className="t-body">
          {t(WAIT_LINE[wait])}
          {wait === "switchedOff" && (
            <>
              {" "}
              <AppLink
                className="link-button"
                href={routeHash(settingsHref("account"))}
              >
                {t("brief.firstDay.turnOn")}
              </AppLink>
            </>
          )}
        </p>
        {openSteps && (
          <>
            <Eyebrow as="h3">{t("brief.firstDay.setup")}</Eyebrow>
            <SetupSteps
              steps={setup.steps}
              count={{ done: setup.done, total: setup.total }}
            />
          </>
        )}
      </PanelBody>
    </Panel>
  );
}
