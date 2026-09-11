// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where the introduction has got to, and who owes the next move.
//
// Four steps, drawn as an ordered list because that is what it is: a reader
// using a screen reader hears "step 2 of 4, current" rather than a row of
// coloured circles. The state is textual, never colour alone.

import { Check } from "lucide-react";

import { useRecordZone } from "../../app/recordzone";
import { Avatar } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { IntroRequest } from "../introrequests";

type StepState = "done" | "current" | "waiting";

/**
 * RelayPanel draws the handoff from the ask to the contact's reply.
 *
 * The fourth step is deliberately not a button. A reply is observed from
 * captured activity, so a control that let somebody declare one would let the
 * record claim an answer the contact never sent.
 */
export function RelayPanel({
  ask,
}: Readonly<{ ask: IntroRequest | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const steps = stepsFor(ask, t);
  const owner = ask && !SETTLED.has(ask.status) ? ownerOf(ask, t) : undefined;
  return (
    <Panel
      title={t("contact.intro.relayTitle")}
      sub={
        ask ? t("contact.intro.relaySubOpen") : t("contact.intro.relaySubNone")
      }
    >
      <PanelBody>
        <ol className="pn-relay">
          {steps.map((step, index) => (
            <li
              className={`pn-relay-step pn-relay-${step.state}`}
              key={step.key}
              aria-current={step.state === "current" ? "step" : undefined}
            >
              <span className="pn-relay-num t-caption" aria-hidden="true">
                {step.state === "done" ? (
                  <Check size={14} />
                ) : (
                  formatNumber(index + 1, locale)
                )}
              </span>
              <div>
                <strong>{t(step.key)}</strong>
                {/* The state in words beside the step, because the ring around
                  the number is colour and colour alone is not a state. */}
                <small>
                  {t(STATE_WORD[step.state])} · {step.detail}
                </small>
              </div>
            </li>
          ))}
        </ol>
        {/* Who owes the next move, named once under the steps rather than
          inferred from which ring is filled. The due date is the colleague's
          answer window and belongs only to the step that has one. */}
        {ask && owner ? (
          <p className="pn-relay-owner">
            <Avatar name={owner} size="sm" />
            <span>{t("contact.intro.handoffOwner", { name: owner })}</span>
            {ask.status === "requested" ? (
              <span className="pn-relay-due t-caption">
                {t("contact.intro.relayDue", {
                  date: formatDate(ask.due_at, locale, recordZone),
                })}
              </span>
            ) : null}
          </p>
        ) : null}
      </PanelBody>
    </Panel>
  );
}

/**
 * ownerOf names who owes the next move on an open ask.
 *
 * Shared with the strip's handoff slot, so the two never disagree about
 * whose turn it is. A status that nobody owes says so rather than naming a
 * contact who has already done their part.
 */
export function ownerOf(ask: IntroRequest, t: ReturnType<typeof useT>): string {
  switch (ask.status) {
    case "requested":
      return ask.introducer_display_name ?? t("contact.intro.ownerColleague");
    case "accepted":
    case "name_drop_approved":
      return ask.requester_display_name ?? t("contact.intro.ownerYou");
    default:
      return t("contact.intro.ownerNobody");
  }
}

type Step = Readonly<{
  key: Parameters<ReturnType<typeof useT>>[0];
  state: StepState;
  detail: string;
}>;

/**
 * stepsFor reads the four steps off one ask.
 *
 * A name-drop settles the third step as a NAME-DROP, never as an introduction:
 * the two are different events, and a relay that showed lent permission as a
 * completed handshake would be the lie this whole feature exists to avoid.
 */
function stepsFor(
  ask: IntroRequest | undefined,
  t: ReturnType<typeof useT>,
): Step[] {
  if (!ask) {
    return [
      {
        key: "contact.intro.stepRoute",
        state: "current",
        detail: t("contact.intro.stepRoutePick"),
      },
      {
        key: "contact.intro.stepRequest",
        state: "waiting",
        detail: t("contact.intro.stepNotSent"),
      },
      {
        key: "contact.intro.stepIntroduction",
        state: "waiting",
        detail: t("contact.intro.stepWaiting"),
      },
      {
        key: "contact.intro.stepReply",
        state: "waiting",
        detail: t("contact.intro.stepObserved"),
      },
    ];
  }
  const settled = SETTLED.has(ask.status);
  const answered = ask.decided_at != null;
  const completed = ask.introduced_at != null || ask.name_dropped_at != null;
  const replied = ask.replied_at != null;

  return [
    {
      key: "contact.intro.stepRoute",
      state: "done",
      detail: ask.introducer_display_name ?? t("contact.intro.ownerColleague"),
    },
    {
      key: "contact.intro.stepRequest",
      state: answered || settled ? "done" : "current",
      detail: answered
        ? t(ANSWER_WORD[ask.status])
        : t("contact.intro.stepAwaitingAnswer"),
    },
    {
      key: ask.name_dropped_at
        ? "contact.intro.stepNameDrop"
        : "contact.intro.stepIntroduction",
      state: completed ? "done" : answered && !settled ? "current" : "waiting",
      detail: completed
        ? t("contact.intro.stepRecorded")
        : t("contact.intro.stepWaiting"),
    },
    {
      key: "contact.intro.stepReply",
      state: replied ? "done" : "waiting",
      detail: t("contact.intro.stepObserved"),
    },
  ];
}

// A status nobody owes anything on. The relay stops rather than pointing at a
// next step that will never come.
const SETTLED = new Set<IntroRequest["status"]>([
  "declined",
  "suggest_other",
  "expired",
  "cancelled",
]);

const STATE_WORD: Record<StepState, Parameters<ReturnType<typeof useT>>[0]> = {
  done: "contact.intro.stepDone",
  current: "contact.intro.stepCurrent",
  waiting: "contact.intro.stepPending",
};

// The answer in words, total over the contract's statuses so a state the
// server can send cannot reach a reader as a raw key.
const ANSWER_WORD: Record<
  IntroRequest["status"],
  Parameters<ReturnType<typeof useT>>[0]
> = {
  requested: "contact.intro.stepAwaitingAnswer",
  accepted: "contact.intro.stateAccepted",
  name_drop_approved: "contact.intro.stateNameDropApproved",
  suggest_other: "contact.intro.stateSuggestOther",
  declined: "contact.intro.stateDeclined",
  introduced: "contact.intro.stateIntroduced",
  name_dropped: "contact.intro.stateNameDropped",
  replied: "contact.intro.stateReplied",
  expired: "contact.intro.stateExpired",
  cancelled: "contact.intro.stateCancelled",
};
