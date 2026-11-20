// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import { Button } from "../design-system/atoms";
import { Checklist, ChecklistStep } from "../design-system/checklist";
import { ConfirmModal } from "../design-system/confirmmodal";
import { StatusBadge } from "../design-system/statusbadge";
import { accountNameOf } from "../format/connectorprovider";
import { formatNumber } from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { deferralDetail } from "../lib/deferral-detail";
import { navigate } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import {
  type SetupStep,
  type SetupStepId,
  stepDials,
} from "../lib/setup-checklist";
import { usePermissionsSeen } from "../lib/setup-permissions";
import {
  VOICE_FOCUS_PARAM,
  type VoiceSetupState,
  voiceFocusFor,
} from "../lib/voice-setup";

const TITLE: Readonly<Record<SetupStepId, MessageKey>> = {
  model: "setup.step.model",
  mail: "setup.step.mail",
  calendar: "setup.step.calendar",
  timezone: "setup.step.timezone",
  language: "setup.step.language",
  permissions: "setup.step.permissions",
  ai: "setup.step.ai",
  voice: "setup.step.writingVoice",
  booking: "setup.step.booking",
};

const ACTION: Readonly<Record<SetupStepId, MessageKey>> = {
  model: "setup.action.model",
  mail: "setup.action.mail",
  calendar: "setup.action.calendar",
  timezone: "setup.action.timezone",
  language: "setup.action.language",
  permissions: "setup.action.permissions",
  ai: "setup.action.ai",
  voice: "setup.action.voice",
  booking: "setup.action.booking",
};

// What a finished step says when no connection names it.
const DONE: Partial<Record<SetupStepId, MessageKey>> = {
  language: "setup.step.languageDone",
  permissions: "setup.step.permissionsDone",
  model: "setup.step.modelDone",
  voice: "setup.step.voiceDone",
};

// What a step says while it is still to do, when no reason overrides it.
const WHY: Readonly<Record<SetupStepId, MessageKey>> = {
  model: "setup.caption.model",
  mail: "setup.caption.mail",
  calendar: "setup.caption.calendar",
  timezone: "setup.caption.timezone",
  language: "setup.caption.language",
  permissions: "setup.caption.permissions",
  ai: "setup.caption.ai",
  voice: "setup.caption.voiceSamples",
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
    caption: "setup.caption.voiceSamples",
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

// A failed or deferred build quotes what the server recorded about it, and a
// thin corpus quotes the server's count of words still missing.
function voiceCaption(t: Translator, locale: Locale, step: SetupStep): string {
  const build = step.voice?.build;
  const recorded =
    build &&
    (step.voice?.state === "failed" || step.voice?.state === "deferred")
      ? deferralDetail(t, build)
      : null;
  const remaining = step.voice?.wordsRemaining;
  if (step.voice?.state === "needsSamples" && remaining) {
    return t("setup.caption.voice", { n: formatNumber(remaining, locale) });
  }
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

function doneCaption(t: Translator, step: SetupStep): string {
  if (step.account) {
    const provider = t(accountNameOf(step.account.provider));
    return step.account.label
      ? t("setup.step.connectedAs", { address: step.account.label, provider })
      : provider;
  }
  if (step.id === "timezone") {
    return step.zone
      ? t("setup.step.timezoneDone", { zone: step.zone })
      : t("setup.step.timezoneDoneUnnamed");
  }
  if (step.id === "ai") {
    const name = step.agent?.name;
    return name
      ? t("setup.step.aiDone", { name })
      : t("setup.step.aiDoneUnnamed");
  }
  return t(DONE[step.id] ?? "setup.step.bookingDone");
}

function useCaption(): (step: SetupStep) => string {
  const t = useT();
  const { locale } = useLocale();
  return (step) => {
    if (step.state === "done") {
      return doneCaption(t, step);
    }
    return step.id === "voice"
      ? voiceCaption(t, locale, step)
      : t(reasonKey(step));
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
  // Kept mounted once done, so the dialog can close and hand focus back.
  if (step.id === "permissions") {
    return (
      <PermissionsAction
        primary={step.state === "current"}
        done={step.state === "done"}
      />
    );
  }
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
      onClick={() => navigate(settingsHref(step.page), stepDials(step.id))}
    >
      {t(
        step.reason === "needsReconnect"
          ? "connectors.reconnect"
          : ACTION[step.id],
      )}
    </Button>
  );
}

// What Margince reads and writes, said in a dialog over wherever the checklist
// is drawn. Pressing "Got it" is what marks the step done.
function PermissionsAction({
  primary,
  done,
}: Readonly<{ primary: boolean; done: boolean }>) {
  const t = useT();
  const { markSeen } = usePermissionsSeen();
  const [open, setOpen] = useState(false);
  // The button is replaced by the done badge on "Got it", so focus returns to
  // this wrapper, which stays.
  const holder = useRef<HTMLSpanElement>(null);
  return (
    <span ref={holder} tabIndex={-1}>
      {done ? (
        <span aria-hidden="true">
          <StatusBadge kind="done" label={t("checklist.stepDone")} />
        </span>
      ) : (
        <Button
          variant={primary ? "primary" : undefined}
          aria-haspopup="dialog"
          onClick={() => setOpen(true)}
        >
          {t("setup.action.permissions")}
        </Button>
      )}
      <ConfirmModal
        returnFocusTo={() => holder.current}
        open={open}
        onClose={() => setOpen(false)}
        title={t("setup.permissions.title")}
        confirmLabel={t("setup.permissions.confirm")}
        cancelLabel={t("setup.permissions.later")}
        onConfirm={() => {
          markSeen();
          setOpen(false);
        }}
      >
        <ul className="form-stack">
          <li>{t("setup.permissions.reads")}</li>
          <li>{t("setup.permissions.writes")}</li>
          <li>{t("setup.permissions.undo")}</li>
        </ul>
      </ConfirmModal>
    </span>
  );
}
