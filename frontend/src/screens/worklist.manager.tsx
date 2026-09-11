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
// They are drawn only on a named contact's queue. On the reader's own day there
// is nobody to reassign work to and nobody to coach.

import { UserRoundArrowLeft } from "lucide-react";
import { useState } from "react";
import {
  Button,
  Field,
  SegmentedControl,
  Textarea,
} from "../design-system/atoms";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useAssignableUserOptions } from "./assigneepicker";
import { problemCodeOf, useMe } from "./common";
import { useRosterPartial, useRosterPartialHint } from "./entityref";
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
// A LABELLED FIELD, and the label is visible. The control carried its name in
// `aria-label` alone, so a sighted reader met a dropdown reading "My own day"
// with nothing on screen saying what it chose — and "My own day" is a value,
// not a question. `Field` puts the words above the control and points the
// control at them, which makes the visible label the accessible name: ONE
// spelling, rather than a label a screen reader hears and a label a reader
// sees being two different strings.
export function OwnerPicker({
  owner,
  onOwner,
}: Readonly<{ owner: string; onOwner: (next: string) => void }>) {
  const t = useT();
  const contacts = useAssignableUserOptions();
  // A ROSTER THAT STOPPED SHORT IS SAID SO. The walk is bounded, so past its
  // reach this list is part of the workspace rather than the workspace — and a
  // picker missing colleagues looks exactly like a workspace that has none, on
  // the one control a lead uses to reach somebody. The words are the roster's
  // own, and the `Field` wires them into the control's `aria-describedby`.
  const partial = useRosterPartial("user", true);
  const partialHint = useRosterPartialHint(partial);
  const options = [{ value: "", label: t("worklist.owner.mine") }, ...contacts];
  return (
    <Field
      label={t("worklist.owner.visibleLabel")}
      hint={partialHint}
      className="worklist-owner-field"
    >
      {(control) => (
        <Select
          {...control}
          options={options}
          value={owner}
          onChange={onOwner}
        />
      )}
    </Field>
  );
}

// Who the reader may hand work to: the contacts, less whoever holds the task
// already. Why the holder is excluded is the caller's own note below.
function useAssigneeOptions(exclude: string | undefined) {
  return useAssignableUserOptions().filter(
    (option) => option.value !== exclude,
  );
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
  // Whose queue is being read, which is the contact a reassignment moves work
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
  // the contact already holding the task. An empty list is the honest state of a
  // question not yet answered; a self-reassignment is a write that changes
  // nothing and looks on screen exactly like one that did.
  const options = useAssigneeOptions(holder);
  const settled = holder !== undefined;
  const reassign = useReassignTask();

  if (!open) {
    // A GLYPH while it is closed. This control sits in a queue row's one line
    // of verbs beside a move, an Open, three judgements and the row's own
    // answer, and the hand-off is the rarest of them — a word for it took the
    // width the reader needed for the work. `IconAction` keeps the name on
    // hover and in the accessible tree, which a bare square would not.
    return (
      <IconAction
        small
        icon={<UserRoundArrowLeft aria-hidden="true" />}
        label={t("worklist.manager.reassign")}
        onClick={() => setOpen(true)}
      />
    );
  }
  // ONE LINE, ending in the answer. The form stands where the row's verbs were,
  // so it keeps their reading: the quiet way out before the call to action, and
  // the call to action on the trailing edge where every other verb on this page
  // puts it. Cancel LAST would sit under the thumb that had been pressing the
  // confirm.
  return (
    <div className="worklist-manager-control">
      <Select
        className="worklist-hand-to"
        options={settled ? options : []}
        value={assignee}
        onChange={setAssignee}
        placeholder={t("worklist.manager.reassignTo")}
        aria-label={t("worklist.manager.reassignTo")}
      />
      <Button variant="ghost" onClick={() => setOpen(false)}>
        {t("worklist.manager.cancel")}
      </Button>
      <Button
        variant="primary"
        disabled={assignee === "" || reassign.isPending}
        onClick={() => {
          reassign.mutate(
            {
              activityId: item.id,
              version: item.version,
              assigneeId: assignee,
            },
            {
              onSuccess: () => {
                setOpen(false);
                toast.show(t("worklist.manager.reassigned"));
              },
              // No completion mark on a refusal: the green dot is the region's
              // way of saying "that worked", and a failure wearing it tells the
              // reader the opposite of what the sentence beside it says.
              onError: () =>
                toast.show(t("worklist.manager.reassignFailed"), {
                  mark: false,
                }),
            },
          );
        }}
      >
        {t("worklist.manager.reassignConfirm")}
      </Button>
    </div>
  );
}

// Leave a note on this contact's queue.
//
// The KIND carries the headline and the note is the coach's own words, which is
// why the kind is a control and the note is a plain field: the recipient reads
// a sentence the product wrote either way, and the coach adds to it rather than
// composing from nothing.
//
// A PANEL, and the same panel in both states. It was a bare ghost button
// floating under the readings that turned into an unchrome'd stack of controls
// on the press — so pressing it changed the page's shape, and the two states
// read as two different features. It is also the reason a lead opened somebody
// else's queue at all, which is why it stands at the head of that day rather
// than among the panels below it.
export function CoachControl({
  owner,
  name,
}: Readonly<{
  owner: string;
  /**
   * Whose day this is, by name, from the roster the page has already read.
   *
   * Absent for a roster that cannot name the id — and for a caller that has no
   * roster at all — which is a real state rather than a loading nicety: a
   * heading reading "A note for undefined" is worse than one that names
   * nobody, and the write itself is addressed by id either way.
   */
  name?: string | null;
}>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<CoachKind>("coach_general");
  const [note, setNote] = useState("");
  const coach = useCoachTeammate();
  const title = name
    ? t("worklist.manager.coachTitle", { name })
    : t("worklist.manager.coachTitleUnnamed");

  if (!open) {
    return (
      <Panel
        className="worklist-coach"
        title={title}
        // What a note DOES, before the reader spends a press finding out. The
        // verb alone said only that something could be left somewhere.
        actions={
          <Button variant="primary" onClick={() => setOpen(true)}>
            {t("worklist.manager.coach")}
          </Button>
        }
      >
        <PanelBody>
          <p className="t-body">{t("worklist.manager.coachIntro")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel
      className="worklist-coach"
      title={title}
      actions={
        <>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            {t("worklist.manager.cancel")}
          </Button>
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
                  onError: (failure) => coachRefused(failure, name, t, toast),
                },
              );
            }}
          >
            {t("worklist.manager.coachConfirm")}
          </Button>
        </>
      }
    >
      {/* `.form-stack` is the tree's own field rhythm — the panel's body is
          padding and nothing else, so two bare fields in one met at the label.
          Not two `PanelBody`s, which would draw the panel's seam hairline
          between the kind and the note: one question, not two sections. */}
      <PanelBody className="form-stack">
        {/* The kind's name is VISIBLE and is also the group's accessible name:
            the fieldset announces the words the row draws, rather than a
            sighted reader meeting four buttons with nothing saying what they
            choose between. Four short labels, all worth seeing at once, which
            is what a segmented strip is for. */}
        <div className="field">
          <span className="t-label">{t("worklist.manager.coachAbout")}</span>
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
        </div>
        <Field label={t("worklist.manager.note")}>
          {(control) => (
            <Textarea
              {...control}
              value={note}
              maxLength={500}
              rows={3}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
      </PanelBody>
    </Panel>
  );
}

/**
 * What a refused note says, and it never wears the completion mark.
 *
 * The server answers 403 `permission_denied` where the caller may not coach
 * this contact at all, which is not a failure to retry — the generic "that could
 * not be left" invited exactly that, and every press earned the same refusal.
 * Read off the RFC-7807 sentinel rather than off the message text: the words
 * are the reader's locale's and the code is the contract's.
 *
 * The named wording needs a name. A roster that cannot name the recipient falls
 * back to the generic sentence, which is still true.
 */
function coachRefused(
  failure: unknown,
  name: string | null | undefined,
  t: ReturnType<typeof useT>,
  toast: ReturnType<typeof useToast>,
): void {
  const refused = problemCodeOf(failure) === "permission_denied";
  toast.show(
    refused && name
      ? t("worklist.manager.coachRefused", { name })
      : t("worklist.manager.coachFailed"),
    { mark: false },
  );
}

/**
 * Take one exception's record for yourself.
 *
 * CONFIRMED, because it moves a record out of somebody's day and into the
 * reader's — the rep who held it loses it from their queue without having
 * pressed anything, so a lead doing it by a misplaced click costs two contacts
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
  // `.cell-actions` is the design system's TABLE-CELL verbs row — right-aligned,
  // one line, no top margin — which is what this cell is. A single verb takes it
  // too: the resting state and the answering one then start at the same x, and
  // the column does not shift under the reader on the press.
  if (!confirming) {
    return (
      <div className="cell-actions">
        <Button
          variant="ghost"
          onClick={(event) => {
            contain(event);
            setConfirming(true);
          }}
        >
          {t("worklist.manager.takeOwnership")}
        </Button>
      </div>
    );
  }
  // The ask is a CAPTION LINE over its own verbs row, not a sentence beside
  // them. Beside them it took the width the two buttons needed and wrapped
  // Cancel to a third line, so answering one row made it twice the height of
  // every other row in the table and put the verb the reader came for below the
  // cell's own fold.
  return (
    <div className="worklist-take">
      <p className="t-caption">{t("worklist.manager.takeOwnershipAsk")}</p>
      <div className="cell-actions">
        <Button
          variant="ghost"
          onClick={(event) => {
            contain(event);
            setConfirming(false);
          }}
        >
          {t("worklist.manager.cancel")}
        </Button>
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
      </div>
    </div>
  );
}
