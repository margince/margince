// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { StatusBadge } from "../design-system/statusbadge";
import { accountNameOf } from "../format/connectorprovider";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { deferralDetail } from "../lib/deferral-detail";
import { navigate } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import type { SetupStep, SetupStepId } from "../lib/setup-checklist";
import {
  VOICE_FOCUS_PARAM,
  type VoiceSetupState,
  voiceFocusFor,
} from "../lib/voice-setup";

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

// The unfinished voice step's caption and verb for each state. An unknown
// state (the read failed) sends the reader to look rather than to add samples.
const VOICE_COPY: Readonly<
  Record<
    Exclude<VoiceSetupState, "active"> | "unknown",
    Readonly<{ caption: MessageKey; action: MessageKey }>
  >
> = {
  review: {
    caption: "setup.caption.voiceReview",
    action: "setup.action.voiceReview",
  },
  building: {
    caption: "setup.caption.voiceBuilding",
    action: "setup.action.voiceProgress",
  },
  deferred: {
    caption: "setup.caption.voiceDeferred",
    action: "setup.action.voiceProgress",
  },
  failed: {
    caption: "setup.caption.voiceFailed",
    action: "setup.action.voiceRetry",
  },
  ready: {
    caption: "setup.caption.voiceReady",
    action: "setup.action.voiceBuild",
  },
  needsSamples: {
    caption: "setup.caption.voice",
    action: "setup.action.voice",
  },
  unknown: {
    caption: "setup.caption.voiceUnknown",
    action: "setup.action.voiceOpen",
  },
};

function voiceCopy(step: SetupStep) {
  const state = step.voice?.state;
  return VOICE_COPY[
    state === undefined || state === "active" ? "unknown" : state
  ];
}

// A failed or deferred build quotes what the server recorded about it.
function voiceCaption(t: Translator, step: SetupStep): string {
  const build = step.voice?.build;
  const recorded =
    build &&
    (step.voice?.state === "failed" || step.voice?.state === "deferred")
      ? deferralDetail(t, build)
      : null;
  const caption = t(voiceCopy(step).caption);
  return recorded ? `${caption} ${recorded}` : caption;
}

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
      return step.id === "voice" ? voiceCaption(t, step) : t(reasonKey(step));
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
  if (step.id === "voice") {
    // The page moves focus to the part the dial names. That holds even when
    // the reader is already on it.
    const focus = voiceFocusFor(step.voice?.state ?? null);
    return (
      <Button
        variant={step.state === "current" ? "primary" : undefined}
        onClick={() =>
          navigate(
            settingsHref(step.page),
            new Map([[VOICE_FOCUS_PARAM, focus]]),
          )
        }
      >
        {t(voiceCopy(step).action)}
      </Button>
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
