// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead looking at somebody else's day.
//
// The Worklist's own rule is that every verb is a LINK to the surface that owns
// it, so the queue adds no authority of its own. These two verbs are the stated
// exception and they keep the rule's REASON: neither implements anything. Both
// call the endpoint the record surface already calls, so there is no second
// place for the rules to live — what is new is only where the reader stands
// when they press it.
//
// They are drawn only on a named person's queue. On the reader's own day there
// is nobody to reassign work to and nobody to coach.

import { useState } from "react";
import { Button, SegmentedControl } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useMe } from "./common";
import { useRoster } from "./entityref";
import {
  subjectAcceptsAnOwner,
  type TeamException,
  useCoachTeammate,
  useReassignTask,
  useTakeOwnership,
  type WorklistItem,
} from "./worklist.queries";

// COACH_KINDS is the contract's vocabulary, spelled once. A kind the server
// gains and this list does not is simply not offered, which is the same
// promise the verb map above the row makes: a control that would refuse when
// pressed is never drawn.
const COACH_KINDS = [
  "coach_reply_aging",
  "coach_deal_needs_next_step",
  "coach_review_backlog",
  "coach_general",
] as const;

type CoachKind = (typeof COACH_KINDS)[number];

// Whose queue this page is answering.
//
// Every colleague, not only teammates: the server decides, and a shorter list
// drawn from a membership read this screen does not have would hide people the
// reader is entitled to open. An ask it refuses answers 403 and the page says
// so, which is the same contract every other control on this page keeps.
export function OwnerPicker({
  owner,
  onOwner,
}: Readonly<{ owner: string; onOwner: (next: string) => void }>) {
  const t = useT();
  const roster = useRoster("user", true);
  const options = [
    { value: "", label: t("worklist.owner.mine") },
    ...(roster.data ?? []).map((entry) => ({
      value: entry.id,
      label: "display_name" in entry ? entry.display_name : entry.id,
    })),
  ];
  return (
    <Select
      options={options}
      value={owner}
      onChange={onOwner}
      aria-label={t("worklist.owner.label")}
    />
  );
}

// Who the reader may hand work to, as options.
//
// AGENT SEATS ARE EXCLUDED. `PATCH /activities/{id}` accepts any active user id
// — its check is existence, not seat kind — so an agent seat would take the
// task and hold it where no person's queue shows it. That is a wider hole than
// this control (issue on the endpoint), and offering the seat here would be
// this page walking a reader into it.
//
// Everyone else the roster carries is offered. Narrowing to teammates would
// need a membership read this screen does not have, and the server refuses a
// reassignment the caller may not make.
function useAssigneeOptions(exclude: string | undefined) {
  const roster = useRoster("user", true);
  return (roster.data ?? [])
    .filter((entry) => entry.id !== exclude)
    .filter((entry) => !("is_agent" in entry && entry.is_agent))
    .map((entry) => ({
      value: entry.id,
      label: "display_name" in entry ? entry.display_name : entry.id,
    }));
}

// Hand one task to somebody else.
//
// The row leaves this queue when the refetch lands, not on the press. The queue
// is ranked and counted server-side — a row removed locally would leave the
// summary above it counting work that is no longer here, and the rank numbers
// beside every remaining row wrong. So the press reports itself through the
// toast, and the list reconciles.
export function ReassignControl({
  item,
  owner,
}: Readonly<{ item: WorklistItem; owner: string }>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [assignee, setAssignee] = useState("");
  // Whose queue is being read, which is the person a reassignment moves work
  // AWAY from. On a rep's drill-down that is the selected rep; on the reader's
  // own queue nobody is selected and it is the reader. Without the fallback the
  // exclusion below compares against "" and matches nobody, so a rep was
  // offered their own name and a press did nothing a reader could see.
  const me = useMe();
  const holder = owner !== "" ? owner : me.data?.user?.id;
  // Nobody is offered until the holder is KNOWN. On the reader's own queue the
  // holder is whoever `/me` names, and until that lands the exclusion below has
  // nothing to compare against — so a picker opened in that window would list
  // the reader's own name, and nothing on the server refuses a reassignment to
  // the person already holding the task. An empty list is the honest state of a
  // question not yet answered; a self-reassignment is a write that changes
  // nothing and looks on screen exactly like one that did.
  const options = useAssigneeOptions(holder);
  const settled = holder !== undefined;
  const reassign = useReassignTask();

  if (!open) {
    return (
      <Button variant="ghost" onClick={() => setOpen(true)}>
        {t("worklist.manager.reassign")}
      </Button>
    );
  }
  return (
    <div className="worklist-manager-control">
      <Select
        options={settled ? options : []}
        value={assignee}
        onChange={setAssignee}
        placeholder={t("worklist.manager.reassignTo")}
        aria-label={t("worklist.manager.reassignTo")}
      />
      <Button
        variant="primary"
        disabled={assignee === "" || reassign.isPending}
        onClick={() => {
          reassign.mutate(
            { activityId: item.id, assigneeId: assignee },
            {
              onSuccess: () => {
                setOpen(false);
                toast.show(t("worklist.manager.reassigned"));
              },
              onError: () => toast.show(t("worklist.manager.reassignFailed")),
            },
          );
        }}
      >
        {t("worklist.manager.reassignConfirm")}
      </Button>
      <Button variant="ghost" onClick={() => setOpen(false)}>
        {t("worklist.manager.cancel")}
      </Button>
    </div>
  );
}

// Leave a note on this person's queue.
//
// The KIND carries the headline and the note is the coach's own words, which is
// why the kind is a control and the note is a plain field: the recipient reads
// a sentence the product wrote either way, and the coach adds to it rather than
// composing from nothing.
export function CoachControl({ owner }: Readonly<{ owner: string }>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<CoachKind>("coach_general");
  const [note, setNote] = useState("");
  const coach = useCoachTeammate();

  if (!open) {
    return (
      <Button variant="ghost" onClick={() => setOpen(true)}>
        {t("worklist.manager.coach")}
      </Button>
    );
  }
  return (
    <div className="worklist-manager-coach">
      <SegmentedControl
        options={COACH_KINDS}
        value={kind}
        onChange={setKind}
        label={t("worklist.manager.coachAbout")}
        labels={{
          coach_reply_aging: t("worklist.manager.kind.reply_aging"),
          coach_deal_needs_next_step: t("worklist.manager.kind.next_step"),
          coach_review_backlog: t("worklist.manager.kind.review_backlog"),
          coach_general: t("worklist.manager.kind.general"),
        }}
      />
      <label className="field">
        <span className="t-label">{t("worklist.manager.note")}</span>
        <textarea
          className="input"
          value={note}
          maxLength={500}
          rows={3}
          onChange={(event) => setNote(event.target.value)}
        />
      </label>
      <div className="worklist-manager-control">
        <Button
          variant="primary"
          disabled={coach.isPending}
          onClick={() => {
            coach.mutate(
              { recipientUserId: owner, kind, note },
              {
                onSuccess: () => {
                  setOpen(false);
                  setNote("");
                  toast.show(t("worklist.manager.coached"));
                },
                onError: () => toast.show(t("worklist.manager.coachFailed")),
              },
            );
          }}
        >
          {t("worklist.manager.coachConfirm")}
        </Button>
        <Button variant="ghost" onClick={() => setOpen(false)}>
          {t("worklist.manager.cancel")}
        </Button>
      </div>
    </div>
  );
}

/**
 * Take one exception's record for yourself.
 *
 * CONFIRMED, because it moves a record out of somebody's day and into the
 * reader's — the rep who held it loses it from their queue without having
 * pressed anything, so a lead doing it by a misplaced click costs two people
 * their sense of what they are carrying.
 *
 * ABSENT rather than disabled where the subject has no owner write. A control
 * that is drawn and then refuses teaches a reader to distrust every control
 * beside it; one that is not drawn says the honest thing, and the row still
 * reaches the record through its own link.
 *
 * It writes through the module that owns the record — the deal's own update
 * for a deal, the activity's for a task — rather than through a worklist
 * writer that would be a second author of a field five modules already audit.
 */
export function TakeOwnershipControl({
  subject,
  viewerId,
  insideAClickableRow = false,
}: Readonly<{
  subject: TeamException["subject"];
  viewerId: string;
  /**
   * Set where the control sits inside a row that navigates on click.
   *
   * The press otherwise fires BOTH: the handover runs and the page walks away
   * from its own confirmation. Stopped on the buttons rather than under a
   * wrapper element, because a handler on a static element is invisible to a
   * keyboard and the a11y lint rejects it.
   */
  insideAClickableRow?: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const [confirming, setConfirming] = useState(false);
  const take = useTakeOwnership();
  const contain = (event: { stopPropagation: () => void }) => {
    if (insideAClickableRow) {
      event.stopPropagation();
    }
  };

  if (!subjectAcceptsAnOwner(subject)) {
    return null;
  }
  if (!confirming) {
    return (
      <Button
        variant="ghost"
        onClick={(event) => {
          contain(event);
          setConfirming(true);
        }}
      >
        {t("worklist.manager.takeOwnership")}
      </Button>
    );
  }
  return (
    <div className="worklist-manager-control">
      <span className="t-label">{t("worklist.manager.takeOwnershipAsk")}</span>
      <Button
        variant="primary"
        disabled={take.isPending}
        onClick={(event) => {
          contain(event);
          take.mutate(
            { subject, userId: viewerId },
            {
              onSuccess: () => {
                setConfirming(false);
                toast.show(t("worklist.manager.tookOwnership"));
              },
              // The refusal stays on screen and the control stays open: a
              // handover that failed leaves the record where it was, and a
              // reader who is not told that believes they now hold it.
              onError: () =>
                toast.show(t("worklist.manager.takeOwnershipFailed"), {
                  mark: false,
                }),
            },
          );
        }}
      >
        {t("worklist.manager.takeOwnershipConfirm")}
      </Button>
      <Button
        variant="ghost"
        onClick={(event) => {
          contain(event);
          setConfirming(false);
        }}
      >
        {t("worklist.manager.cancel")}
      </Button>
    </div>
  );
}
