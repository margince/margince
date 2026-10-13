// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge, Button } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { Heading } from "../design-system/heading";
import { Popover } from "../design-system/popover";
import { Meter } from "../design-system/readings";
import { accountNameOf } from "../format/connectorprovider";
import { type Translator, useT } from "../i18n";
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
}: Readonly<{ steps: readonly SetupStep[] }>) {
  const t = useT();
  const caption = useCaption();
  return (
    <Checklist>
      {steps.map((step) => (
        <ChecklistStep
          key={step.id}
          state={step.state}
          title={t(TITLE[step.id])}
          caption={caption(step)}
          action={<StepAction step={step} />}
        />
      ))}
    </Checklist>
  );
}

// Every outcome sits in one cell of one width, so the rows line up; the mark's
// ring says which step is next, never a louder button.
function StepAction({ step }: Readonly<{ step: SetupStep }>) {
  const t = useT();
  return <span className="setup-step-action">{stepActionOf(step, t)}</span>;
}

function stepActionOf(step: SetupStep, t: Translator) {
  if (step.state === "done") {
    // The step title already carries the state as a word for a screen reader.
    return (
      <span aria-hidden="true">
        <Badge tone="success">{t("checklist.stepDone")}</Badge>
      </span>
    );
  }
  if (step.blocked) {
    // The caption names what it waits on; this only holds the cell.
    return (
      <span className="setup-step-waiting" aria-hidden="true">
        {t("setup.reminder.waiting")}
      </span>
    );
  }
  return (
    <Button onClick={() => navigate(settingsHref(step.page))}>
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
  const setup = useSetupChecklist(useSettingsReach());
  if (
    !setup.settled ||
    setup.hidden ||
    setup.total === 0 ||
    setup.done === setup.total
  ) {
    return null;
  }
  const count = { done: String(setup.done), total: String(setup.total) };
  return (
    <Popover
      className="setup-reminder"
      label={
        <>
          <span className="setup-reminder-dot" aria-hidden="true" />
          <span className="setup-reminder-word">
            {t("setup.reminder.trigger")}
          </span>
          <span className="setup-reminder-count">
            {t("setup.reminder.count", count)}
          </span>
        </>
      }
    >
      <div className="setup-reminder-panel">
        <Heading size="xsmall" as="h2" className="setup-reminder-title">
          {t("setup.reminder.title")}
        </Heading>
        <div className="setup-reminder-progress">
          <Meter
            value={setup.done}
            max={setup.total}
            label={t("setup.reminder.progress")}
            dense
          />
          <span className="t-caption">
            {t("settings.home.setup.count", count)}
          </span>
        </div>
        <SetupSteps steps={setup.steps} />
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
