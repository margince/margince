// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Circle, CircleCheck, CircleX } from "lucide-react";
import type { components } from "../api/schema";
import { Badge, BusyMark, Button } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { type Fact, FactList } from "../design-system/factlist";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody } from "../design-system/panel";
import { type Translator, useT } from "../i18n";
import { PROVIDER_VIDEO_APP, VIDEO_APP_NAME } from "./booking-video";
import { INVITATION_IN_FLIGHT } from "./meeting-invitation-query";

type Invitation = components["schemas"]["MeetingInvitation"];
type Provider = NonNullable<Invitation["provider"]>;
type StepState = "done" | "active" | "failed" | "waiting";
type Step = Readonly<{ key: string; state: StepState; label: string }>;

// The provider names the calendar that holds the event, and so the product a
// host opens it in.
const OPEN_CALENDAR: Readonly<
  Record<
    Provider,
    "scheduling.openGoogleCalendar" | "scheduling.openOutlookCalendar"
  >
> = {
  gcal: "scheduling.openGoogleCalendar",
  graphcal: "scheduling.openOutlookCalendar",
};

export function openCalendarLabel(t: Translator, provider?: Provider) {
  return t(provider ? OPEN_CALENDAR[provider] : "scheduling.openCalendar");
}

const STATUS_TONE: Readonly<
  Record<Invitation["status"], "info" | "success" | "warning" | "default">
> = {
  pending: "info",
  confirmed: "success",
  needs_attention: "warning",
  rescheduling: "info",
  canceling: "info",
  canceled: "default",
};

/** Where the calendar invitation stands, in the words the meeting page uses. */
export function InvitationBadge({
  status,
}: Readonly<{ status: Invitation["status"] }>) {
  const t = useT();
  return (
    <Badge tone={STATUS_TONE[status]} live={INVITATION_IN_FLIGHT.has(status)}>
      {t(`scheduling.${status}`)}
    </Badge>
  );
}

// The step's words carry its state; the mark is there to be scanned.
function StepMark({ state }: Readonly<{ state: StepState }>) {
  if (state === "active") return <BusyMark />;
  const Icon = { done: CircleCheck, failed: CircleX, waiting: Circle }[state];
  return <Icon aria-hidden="true" />;
}

// What the calendar has done with the invitation. The guest's own answer is a
// separate step on purpose: a calendar that accepted the event has said nothing
// about whether the guest will come.
function deliverySteps(status: Invitation["status"], t: Translator): Step[] {
  const calendar: Record<Invitation["status"], Step> = {
    pending: {
      key: "calendar",
      state: "active",
      label: t("scheduling.step.sending"),
    },
    rescheduling: {
      key: "calendar",
      state: "active",
      label: t("scheduling.step.sendingChange"),
    },
    canceling: {
      key: "calendar",
      state: "active",
      label: t("scheduling.step.sendingCancel"),
    },
    confirmed: {
      key: "calendar",
      state: "done",
      label: t("scheduling.step.accepted"),
    },
    needs_attention: {
      key: "calendar",
      state: "failed",
      label: t("scheduling.step.refused"),
    },
    canceled: {
      key: "calendar",
      state: "done",
      label: t("scheduling.step.canceled"),
    },
  };
  const steps: Step[] = [
    { key: "created", state: "done", label: t("scheduling.step.created") },
    calendar[status],
  ];
  // Only a calendar that took the invitation leaves the guest something to answer.
  if (!["canceling", "canceled", "needs_attention"].includes(status))
    steps.push({
      key: "reply",
      state: "waiting",
      label: t("scheduling.step.reply"),
    });
  return steps;
}

/** The host's side card: where the invitation is on its way to the guest. */
export function DeliveryCard({
  status,
}: Readonly<{ status: Invitation["status"] }>) {
  const t = useT();
  const steps = deliverySteps(status, t);
  const current = steps.find((step) => step.state !== "done");
  return (
    <Panel title={t("scheduling.deliveryTitle")}>
      <PanelBody>
        <ol className="bookmeet-steps">
          {steps.map((step) => (
            <li
              key={step.key}
              className={`bookmeet-step bookmeet-step-${step.state}`}
              aria-current={step === current ? "step" : undefined}
            >
              <StepMark state={step.state} />
              <span>{step.label}</span>
            </li>
          ))}
        </ol>
        <p className="t-caption bookmeet-help">
          {t("scheduling.deliveryHelp")}
        </p>
      </PanelBody>
    </Panel>
  );
}

// The video row says what is true about the link right now: here it is, it is
// coming once the calendar answers, or the calendar answered without one.
function videoFact(
  meeting: Invitation,
  host: boolean,
  t: Translator,
  copy: ReturnType<typeof useClipboardCopy>,
): Fact | undefined {
  const app = meeting.provider
    ? VIDEO_APP_NAME[PROVIDER_VIDEO_APP[meeting.provider]]
    : undefined;
  const term = t("scheduling.fact.video");
  if (meeting.video_url)
    return {
      key: "video",
      term,
      value: (
        <span className="bookmeet-video">
          {app && <span>{app}</span>}
          <OffsiteLink href={meeting.video_url}>
            {meeting.video_url}
          </OffsiteLink>
          <Button onClick={copy.copy}>{copy.label}</Button>
        </span>
      ),
    };
  if (
    !meeting.video_call ||
    meeting.status === "canceled" ||
    meeting.status === "canceling"
  )
    return undefined;
  if (meeting.status === "confirmed")
    return host
      ? {
          key: "video",
          term,
          value: app ?? term,
          note: t("scheduling.videoMissing"),
        }
      : undefined;
  return {
    key: "video",
    term,
    value: app ?? term,
    note: t("scheduling.videoPending"),
  };
}

/** Who, where and how: only the facts this meeting actually has. */
export function MeetingFacts({
  meeting,
  host,
}: Readonly<{ meeting: Invitation; host: boolean }>) {
  const t = useT();
  const copy = useClipboardCopy(meeting.video_url ?? "", {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  const reminder =
    host &&
    meeting.status === "confirmed" &&
    meeting.reminder_status &&
    meeting.reminder_status !== "off"
      ? meeting.reminder_status
      : undefined;
  const facts = [
    videoFact(meeting, host, t, copy),
    meeting.location.trim()
      ? {
          key: "location",
          term: t("scheduling.fact.location"),
          value: meeting.location,
        }
      : undefined,
    reminder
      ? {
          key: "reminder",
          term: t("scheduling.fact.reminder"),
          value: t(`scheduling.reminder.${reminder}`),
        }
      : undefined,
  ].filter((fact): fact is Fact => fact !== undefined);
  if (facts.length === 0) return null;
  return (
    <>
      <FactList facts={facts} />
      {copy.notice}
    </>
  );
}
