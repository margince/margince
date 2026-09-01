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
import { useRoster } from "./entityref";
import {
  useCoachTeammate,
  useReassignTask,
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
// The roster is the workspace's, and the server refuses a reassignment the
// caller may not make. Drawing only teammates here would need a membership read
// this screen does not have, and guessing at one would draw a shorter list than
// the reader is entitled to.
function useAssigneeOptions(exclude: string | undefined) {
  const roster = useRoster("user", true);
  return (roster.data ?? [])
    .filter((entry) => entry.id !== exclude)
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
  const options = useAssigneeOptions(owner);
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
        options={options}
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
        <span className="t-meta">{t("worklist.manager.note")}</span>
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
