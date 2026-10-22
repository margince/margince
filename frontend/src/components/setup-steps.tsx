// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { StatusBadge } from "../design-system/statusbadge";
import { accountNameOf } from "../format/connectorprovider";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { navigate } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import type { SetupStep, SetupStepId } from "../lib/setup-checklist";

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
