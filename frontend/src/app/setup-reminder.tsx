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
import type { Route } from "../lib/router";
import type { SettingsReach } from "../lib/settingscatalog";
import { settingsHref } from "../lib/settingsrouting";
import { type SetupChecklist, useSetupChecklist } from "../lib/setup-checklist";
import "./setup-reminder.css";
import "./topbar-popover.css";

const NO_REACH: SettingsReach = { acts: [], looksUp: [] };

/**
 * The checklist while the reminder has something to ask, otherwise null. With
 * `enabled` false it reads nothing, for a caller that shows it at one width.
 */
export function useSetupReminder(enabled = true): SetupChecklist | null {
  const reach = useSettingsReach();
  const setup = useSetupChecklist(enabled ? reach : NO_REACH);
  if (
    !setup.settled ||
    setup.hidden ||
    setup.total === 0 ||
    setup.done === setup.total
  ) {
    return null;
  }
  return setup;
}

/**
 * Where the reminder leads without its panel: the page of the step to do next.
 * While every open step waits on someone else, it is the first open one.
 */
export function nextStepRoute(setup: SetupChecklist): Route {
  const next =
    setup.steps.find((step) => step.state === "current") ??
    setup.steps.find((step) => step.state !== "done");
  return settingsHref(next?.page);
}

/** "Finish setup" and its count, as the trigger and the account menu print it. */
export function SetupReminderLabel({
  setup,
}: Readonly<{ setup: SetupChecklist }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <span className="setup-reminder-word">{t("setup.reminder.trigger")}</span>
      <Badge>
        {t("setup.reminder.count", {
          done: formatNumber(setup.done, locale),
          total: formatNumber(setup.total, locale),
        })}
      </Badge>
    </>
  );
}

/**
 * "Finish setup 2 of 4" in the top bar, until every step is done or the reader
 * hides it for this sign-in. Rail-less routes draw no top bar, so no reminder.
 * At phone width the account menu carries it instead (app/account.tsx).
 */
export function SetupReminder({
  onHide,
}: Readonly<{
  /** Takes focus before the reminder, and the button pressed in it, unmount. */
  onHide: () => void;
}>) {
  const t = useT();
  const setup = useSetupReminder();
  if (!setup) {
    return null;
  }
  return (
    <Popover
      className="setup-reminder"
      variant="ghost"
      width="wide"
      pinned
      panelClassName="topbar-popover setup-reminder-popover"
      label={<SetupReminderLabel setup={setup} />}
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
