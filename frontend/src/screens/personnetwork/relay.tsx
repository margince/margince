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
      title={t("person.intro.relayTitle")}
      sub={
        ask ? t("person.intro.relaySubOpen") : t("person.intro.relaySubNone")
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
              <span className="pn-relay-num" aria-hidden="true">
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
            <span>{t("person.intro.handoffOwner", { name: owner })}</span>
            {ask.status === "requested" ? (
              <span className="pn-relay-due">
                {t("person.intro.relayDue", {
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
 * person who has already done their part.
 */
export function ownerOf(ask: IntroRequest, t: ReturnType<typeof useT>): string {
  switch (ask.status) {
    case "requested":
      return ask.introducer_display_name ?? t("person.intro.ownerColleague");
    case "accepted":
    case "name_drop_approved":
      return ask.requester_display_name ?? t("person.intro.ownerYou");
    default:
      return t("person.intro.ownerNobody");
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
        key: "person.intro.stepRoute",
        state: "current",
        detail: t("person.intro.stepRoutePick"),
      },
      {
        key: "person.intro.stepRequest",
        state: "waiting",
        detail: t("person.intro.stepNotSent"),
      },
      {
        key: "person.intro.stepIntroduction",
        state: "waiting",
        detail: t("person.intro.stepWaiting"),
      },
      {
        key: "person.intro.stepReply",
        state: "waiting",
        detail: t("person.intro.stepObserved"),
      },
    ];
  }
  const settled = SETTLED.has(ask.status);
  const answered = ask.decided_at != null;
  const completed = ask.introduced_at != null || ask.name_dropped_at != null;
  const replied = ask.replied_at != null;

  return [
    {
      key: "person.intro.stepRoute",
      state: "done",
      detail: ask.introducer_display_name ?? t("person.intro.ownerColleague"),
    },
    {
      key: "person.intro.stepRequest",
      state: answered || settled ? "done" : "current",
      detail: answered
        ? t(ANSWER_WORD[ask.status])
        : t("person.intro.stepAwaitingAnswer"),
    },
    {
      key: ask.name_dropped_at
        ? "person.intro.stepNameDrop"
        : "person.intro.stepIntroduction",
      state: completed ? "done" : answered && !settled ? "current" : "waiting",
      detail: completed
        ? t("person.intro.stepRecorded")
        : t("person.intro.stepWaiting"),
    },
    {
      key: "person.intro.stepReply",
      state: replied ? "done" : "waiting",
      detail: t("person.intro.stepObserved"),
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
  done: "person.intro.stepDone",
  current: "person.intro.stepCurrent",
  waiting: "person.intro.stepPending",
};

// The answer in words, total over the contract's statuses so a state the
// server can send cannot reach a reader as a raw key.
const ANSWER_WORD: Record<
  IntroRequest["status"],
  Parameters<ReturnType<typeof useT>>[0]
> = {
  requested: "person.intro.stepAwaitingAnswer",
  accepted: "person.intro.stateAccepted",
  name_drop_approved: "person.intro.stateNameDropApproved",
  suggest_other: "person.intro.stateSuggestOther",
  declined: "person.intro.stateDeclined",
  introduced: "person.intro.stateIntroduced",
  name_dropped: "person.intro.stateNameDropped",
  replied: "person.intro.stateReplied",
  expired: "person.intro.stateExpired",
  cancelled: "person.intro.stateCancelled",
};
