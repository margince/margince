// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Button } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { Heading } from "../design-system/heading";
import { Popover } from "../design-system/popover";
import { Meter } from "../design-system/readings";
import { StatusBadge } from "../design-system/statusbadge";
import { accountNameOf } from "../format/connectorprovider";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { navigate } from "../lib/router";
import { useSettingsReach } from "../screens/settingsnav";
import { settingsHref } from "../screens/settingsrouting";
import {
  type SetupStep,
  type SetupStepId,
  useSetupChecklist,
} from "./setup-checklist";
import "./setup-reminder.css";

const TITLE: Readonly<Record<SetupStepId, MessageKey>> = {
  mail: "setup.step.mail",
  calendar: "setup.step.calendar",
  voice: "setup.step.writingVoice",
  booking: "setup.step.booking",
};

const ACTION: Readonly<Record<SetupStepId, MessageKey>> = {
  mail: "setup.action.mail",
  calendar: "setup.action.calendar",
  voice: "setup.action.voice",
  booking: "setup.action.booking",
};

// What a step says while it is still to do, when no reason overrides it.
const WHY: Readonly<Record<SetupStepId, MessageKey>> = {
  mail: "setup.caption.mail",
  calendar: "setup.caption.calendar",
  voice: "setup.caption.voice",
  booking: "setup.caption.booking",
};

const REASON: Readonly<
  Record<Exclude<SetupStep["reason"], "needsReconnect" | null>, MessageKey>
> = {
  skipped: "setup.caption.calendarSkipped",
  needsVendor: "setup.caption.calendarNeedsVendor",
  waitsAdmin: "setup.caption.waitsAdmin",
  needsCalendar: "setup.caption.bookingNeedsCalendar",
};

function reasonKey(step: SetupStep): MessageKey {
  if (step.reason === "needsReconnect") {
    return step.id === "mail"
      ? "setup.caption.mailReconnect"
      : "setup.caption.calendarReconnect";
  }
  return step.reason ? REASON[step.reason] : WHY[step.id];
}

function useCaption(): (step: SetupStep) => string {
  const t = useT();
  return (step) => {
    if (step.state !== "done") {
      return t(reasonKey(step));
    }
    if (step.account) {
      const provider = t(accountNameOf(step.account.provider));
      return step.account.label
        ? t("setup.step.connectedAs", { address: step.account.label, provider })
        : provider;
    }
    return t(
      step.id === "voice" ? "setup.step.voiceDone" : "setup.step.bookingDone",
    );
  };
}

/** The steps as the reminder and the settings home both draw them. */
export function SetupSteps({
  steps,
  count,
}: Readonly<{
  steps: readonly SetupStep[];
  count?: Readonly<{ done: number; total: number }>;
}>) {
  const t = useT();
  const caption = useCaption();
  return (
    <Checklist count={count}>
      {steps.map((step) => {
        const title = t(TITLE[step.id]);
        // A blocked step waits on somebody else: the caption names who.
        if (
          step.state === "waiting" ||
          (step.blocked && step.state !== "done")
        ) {
          return (
            <ChecklistStep
              key={step.id}
              state="waiting"
              title={title}
              caption={caption(step)}
            />
          );
        }
        return (
          <ChecklistStep
            key={step.id}
            state={step.state}
            title={title}
            caption={caption(step)}
            action={<StepAction step={step} />}
          />
        );
      })}
    </Checklist>
  );
}

function StepAction({ step }: Readonly<{ step: SetupStep }>) {
  const t = useT();
  if (step.state === "done") {
    // The step title already carries the state as a word for a screen reader.
    return (
      <span aria-hidden="true">
        <StatusBadge kind="done" label={t("checklist.stepDone")} />
      </span>
    );
  }
  return (
    <Button
      variant={step.state === "current" ? "primary" : undefined}
      onClick={() => navigate(settingsHref(step.page))}
    >
      {t(
        step.reason === "needsReconnect"
          ? "connectors.reconnect"
          : ACTION[step.id],
      )}
    </Button>
  );
}

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
