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

import { useState } from "react";
import { useAssignableUserOptions } from "../../components/assigneepicker";
import {
  RosterFailedLine,
  useRosterHint,
  useRosterPartial,
} from "../../components/entityref";
import {
  type AssignedRecord,
  assignedRecord,
} from "../../components/recordassignments.queries";
import { Button, Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { useMe } from "../../lib/session";
import { InquiryAssignmentDialog } from "./worklist.inquiryassignment";
import {
  subjectAcceptsAnOwner,
  type TeamException,
  useReassignTask,
  useTakeOwnership,
  type WorklistItem,
  type WorklistScope,
} from "./worklist.queries";

// Whose queue this page is answering.
//
// A LABELLED FIELD, and the label is visible. The control carried its name in
// `aria-label` alone, so a sighted reader met a dropdown reading "My own day"
// with nothing on screen saying what it chose — and "My own day" is a value,
// not a question. `Field` puts the words above the control and points the
// control at them, which makes the visible label the accessible name: ONE
// spelling, rather than a label a screen reader hears and a label a reader
// sees being two different strings.
const NOBODY_NAMED = {
  mine: "worklist.owner.mine",
  team: "worklist.owner.wholeTeam",
  all: "worklist.owner.everyone",
  unassigned: "worklist.owner.nobodyYet",
} as const satisfies Record<WorklistScope, MessageKey>;

export function OwnerPicker({
  owner,
  onOwner,
  scope,
}: Readonly<{
  owner: string;
  onOwner: (next: string) => void;
  scope: WorklistScope;
}>) {
  const t = useT();
  const { options: contacts, roster } = useAssignableUserOptions();
  // A picker missing colleagues looks like a workspace that has none, so a
  // roster that failed or stopped short says so. `Field` wires the hint into
  // the control's `aria-describedby`.
  const rosterHint = useRosterHint(roster, useRosterPartial("user", true));
  // Naming nobody follows the scope dial, so the empty choice says which
  // population that is; "My Worklist" over the whole team misstates it.
  const options = [
    { value: "", label: t(labelFor(NOBODY_NAMED, scope)) },
    ...contacts,
  ];
  return (
    <Field
      label={t("worklist.owner.visibleLabel")}
      hint={rosterHint}
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
// already, with those named on the task's record first. Why the holder is
// excluded is the caller's own note below.
function useAssigneeOptions(
  exclude: string | undefined,
  records: readonly AssignedRecord[],
) {
  const { options, roster } = useAssignableUserOptions(true, records);
  return {
    options: options.filter((option) => option.value !== exclude),
    roster,
  };
}

// Hand one task to somebody else, in a form the row opens from its menu. The row
// leaves when the refetch lands, not on the press. The server ranks and counts
// the queue, so a row removed here would leave the summary and ranks wrong.
export function ReassignForm(
  props: Readonly<{ item: WorklistItem; owner: string; onClose: () => void }>,
) {
  return props.item.kind === "website_inquiry" ? (
    <InquiryAssignmentDialog
      item={props.item}
      mode="assign_and_follow_up"
      onClose={props.onClose}
    />
  ) : (
    <TaskReassignForm {...props} />
  );
}

function TaskReassignForm({
  item,
  owner,
  onClose,
}: Readonly<{ item: WorklistItem; owner: string; onClose: () => void }>) {
  const t = useT();
  const toast = useToast();
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
  const { options, roster } = useAssigneeOptions(
    holder,
    assignedRecord(item.subject?.type, item.subject?.id),
  );
  const settled = holder !== undefined;
  const reassign = useReassignTask();

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
      <Button variant="ghost" onClick={onClose}>
        {t("worklist.manager.cancel")}
      </Button>
      <Button
        variant="primary"
        disabled={assignee === "" || reassign.isPending}
        onClick={() => {
          // mutateAsync, as Take it does: the refetch can drop the row, and
          // its form with it, before per-call callbacks would run.
          reassign
            .mutateAsync({
              activityId: item.id,
              version: item.version,
              assigneeId: assignee,
            })
            .then(
              () => {
                onClose();
                toast.show(t("worklist.manager.reassigned"));
              },
              // No completion mark on a refusal: the green dot is the region's
              // way of saying "that worked", and a failure wearing it tells the
              // reader the opposite of what the sentence beside it says.
              () =>
                toast.show(t("worklist.manager.reassignFailed"), {
                  tone: "danger",
                }),
            );
        }}
      >
        {t("worklist.manager.reassignConfirm")}
      </Button>
      <RosterFailedLine kind="user" roster={roster} />
    </div>
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
      <p>{t("worklist.manager.takeOwnershipAsk")}</p>
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
                    tone: "danger",
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
