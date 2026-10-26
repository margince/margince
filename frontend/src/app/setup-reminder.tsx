// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SetupSteps } from "../components/setup-steps";
import { Badge, Button } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Popover } from "../design-system/popover";
import { Meter } from "../design-system/readings";
import { useSettingsReach } from "../features/settings/settingsnav";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useSetupChecklist } from "../lib/setup-checklist";
import "./setup-reminder.css";

/**
 * "Finish setup 2 of 4" in the top bar, until every step is done or the reader
 * hides it for this sign-in. Rail-less routes draw no top bar, so no reminder.
 */
export function SetupReminder({
  onHide,
}: Readonly<{
  /** Takes focus before the reminder, and the button pressed in it, unmount. */
  onHide: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const setup = useSetupChecklist(useSettingsReach());
  if (
    !setup.settled ||
    setup.hidden ||
    setup.total === 0 ||
    setup.done === setup.total
  ) {
    return null;
  }
  return (
    <Popover
      className="setup-reminder"
      variant="ghost"
      width="wide"
      label={
        <>
          <span className="setup-reminder-word">
            {t("setup.reminder.trigger")}
          </span>
          <Badge>
            {t("setup.reminder.count", {
              done: formatNumber(setup.done, locale),
              total: formatNumber(setup.total, locale),
            })}
          </Badge>
        </>
      }
    >
      <div className="setup-reminder-panel">
        <Heading size="xsmall" as="h2" className="setup-reminder-title">
          {t("setup.reminder.title")}
        </Heading>
        <Meter
          value={setup.done}
          max={setup.total}
          label={t("setup.reminder.progress")}
          dense
        />
        <SetupSteps
          steps={setup.steps}
          count={{ done: setup.done, total: setup.total }}
        />
        <div className="setup-reminder-foot">
          <Button
            variant="link"
            onClick={() => {
              onHide();
              setup.hide();
            }}
          >
            {t("setup.reminder.hide")}
          </Button>
        </div>
      </div>
    </Popover>
  );
}
