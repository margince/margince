// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal card, worklist and record history share task actions so completion
// and Undo follow the same write path wherever a task is opened.

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { ListChecks } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useActivityWritable } from "../../components/activitygate";
import { useTaskUpdate } from "../../components/taskactions";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { refreshSuggestedMoves, useCurrentMove } from "../../lib/currentmove";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { ContactMeetingBrief } from "../brief/meetingbrief";
import { ComposeModal } from "../mail/compose";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { TaskDetailModal } from "../worklist/taskdetailmodal";

// What a move must carry to be performed. Both producers' shapes satisfy it.
export type PerformableMove = {
  action: string;
  recommendation?: components["schemas"]["RecommendationReceipt"];
  arguments?: Record<string, unknown>;
};

// activityIdOf reads the one operand the meeting-brief verb takes. The
// arguments object is typed open on the wire; a missing id renders no button
// rather than a button that would 404.
export function activityIdOf(move: PerformableMove): string | null {
  const raw = move.arguments?.activity_id;
  return typeof raw === "string" && raw !== "" ? raw : null;
}

// recipientsOf reads who a follow-up move writes to: the contact links its
// arguments carry, best first. Anything else in the list names nobody.
export function recipientsOf(move: PerformableMove): string[] {
  const links = move.arguments?.links;
  if (!Array.isArray(links)) return [];
  return links.flatMap((link: unknown) => {
    if (typeof link !== "object" || link === null) return [];
    const { entity_type: kind, entity_id: id } = link as Record<
      string,
      unknown
    >;
    return kind === "contact" && typeof id === "string" && id !== ""
      ? [id]
      : [];
  });
}

// Whether the verb has what it needs to be drawn, decided BEFORE the slot is:
// a slot handed a control that renders nothing still draws the slot, and an
// empty action region reads as a verb that failed to load. MoveButton keeps
// its own null returns for the narrowing the cases need; this is what decides
// whether the row has a verb at all.
export function hasMoveControl(move: PerformableMove): boolean {
  switch (move.action) {
    case "create_task":
      return Boolean(move.arguments);
    case "open_task":
    case "open_meeting_brief":
      return activityIdOf(move) !== null;
    case "draft_email":
      return activityIdOf(move) !== null && recipientsOf(move).length > 0;
    default:
      return false;
  }
}

export function MoveButton({
  dealId,
  contactId,
  move,
  taskRefreshKeys = [],
}: Readonly<{
  // The deal whose card to refresh after the verb writes, where the caller has
  // one. OPTIONAL because the worklist does not: a queue row's move may belong
  // to a deal, a contact or nothing at all, and passing an empty string would
  // invalidate a key naming no deal — a call that does nothing, spelled as one
  // that does something.
  dealId?: string;
  contactId?: string;
  move: PerformableMove;
  taskRefreshKeys?: readonly QueryKey[];
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [briefId, setBriefId] = useState<string | null>(null);
  // This button's own drawer. Every other host of the meeting brief already
  // mounts one for its timeline; this one is a lone control on the deal card,
  // so a message cited in the brief it opens has nowhere else to go.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useViewerZone();
  const [taskId, setTaskId] = useState<string | null>(null);
  const openTask = useCurrentMove<string>(setTaskId);
  const openBrief = useCurrentMove<string>(setBriefId);
  const canUpdateTask = useCanWrite("activity", "update");
  const taskUpdate = useTaskUpdate([
    ...taskRefreshKeys,
    ...(dealId ? [["deal-status", dealId]] : []),
    ...(contactId ? [["contact360", contactId]] : []),
    ["tasks"],
    ["worklist"],
  ]);
  const activityId = activityIdOf(move);
  const createTask = useMutation({
    mutationKey: ["deal-status-create-task"],
    // The arguments ARE the task body the server prepared; the click sends
    // them as they came rather than re-deriving them from render state.
    mutationFn: async (body: Record<string, unknown>) => {
      return unwrap(
        await api.POST("/tasks", {
          body: body as components["schemas"]["CreateTaskRequest"],
        }),
        t,
      );
    },
    onError: () => refreshSuggestedMoves(queryClient),
    onSuccess: () => {
      if (dealId) {
        queryClient.invalidateQueries({ queryKey: ["deal-status", dealId] });
      }
      queryClient.invalidateQueries({ queryKey: ["activities"] });
      queryClient.invalidateQueries({ queryKey: ["tasks"] });
      // And the queue, which is now one of the two surfaces this button is
      // pressed from. The deal card refreshes itself above; the worklist row
      // that ASKED for this task would otherwise keep asking for it, because
      // the queue does not poll. The open_task arm gets this for free from
      // useTaskUpdate's own key list; this arm has its own onSuccess and so
      // needs it spelled.
      queryClient.invalidateQueries({ queryKey: ["worklist"] });
      // And every contact page, because this button can now file work AGAINST a
      // contact: the meeting move links the task to the stakeholder it names.
      // That contact's page opens on a moment derived from exactly this task,
      // so leaving its cache alone means a reader who files the meeting here
      // and clicks through still reads "no next step with them" for the next
      // half-minute. The key is per contact and this button does not know
      // which, so the prefix is invalidated whole — one page's worth of refetch
      // against telling a reader something that stopped being true when they
      // pressed the button.
      queryClient.invalidateQueries({ queryKey: ["contact360"] });
    },
  });

  const taskBody = move.arguments;
  // The task the press wrote stays named here, with the way into it, even
  // after the refreshed advice moves on. The reader sees what they made.
  if (createTask.data) {
    return (
      <CreatedFollowUp
        taskId={createTask.data.id}
        readOnly={!canUpdateTask}
        update={taskUpdate}
      />
    );
  }
  switch (move.action) {
    case "open_task":
      if (!activityId) return null;
      return (
        <>
          <Button
            pending={openTask.isPending}
            onClick={() =>
              openTask.mutate({
                receipt: move.recommendation,
                operand: activityId,
              })
            }
          >
            {t("deal360.openTask")}
          </Button>
          <ErrorLine error={openTask.error} />
          {taskId && (
            <TaskDetailModal
              activityId={taskId}
              readOnly={!canUpdateTask}
              onClose={() => setTaskId(null)}
              update={taskUpdate}
            />
          )}
        </>
      );
    case "create_task":
      return (
        <CreateTaskVerb
          body={taskBody}
          pending={createTask.isPending}
          error={createTask.error}
          onCreate={createTask.mutate}
        />
      );
    case "draft_email":
      // Only the follow-up after a meeting draws a button: it names who the
      // meeting was with, which the deal's email box cannot know. Any other
      // mail move is the email box's job, the one place a rep writes from.
      if (!activityId || recipientsOf(move).length === 0) {
        return null;
      }
      return (
        <FollowUpDraft
          dealId={dealId}
          meetingId={activityId}
          recipients={recipientsOf(move)}
          recommendation={move.recommendation}
        />
      );
    case "open_meeting_brief":
      if (!activityId) {
        return null;
      }
      return (
        <>
          <Button
            variant="ai"
            pending={openBrief.isPending}
            onClick={() =>
              openBrief.mutate({
                receipt: move.recommendation,
                operand: activityId,
              })
            }
          >
            {t("deal360.openBrief")}
          </Button>
          <ErrorLine error={openBrief.error} />
          <ContactMeetingBrief
            activityId={briefId ?? activityId}
            open={briefId !== null}
            onClose={() => setBriefId(null)}
            onOpenEmail={setOpenEmail}
          />
          <OpenEmailDrawer
            activityId={openEmail}
            zone={zone}
            onClose={() => setOpenEmail(null)}
          />
        </>
      );
    default:
      return null;
  }
}

// The composer, opened to everyone a meeting was with. Grounded in the first
// of them, with the rest on the To line beside them.
function FollowUpDraft({
  dealId,
  meetingId,
  recipients,
  recommendation,
}: Readonly<{
  recommendation?: components["schemas"]["RecommendationReceipt"];
  dealId?: string;
  meetingId: string;
  recipients: readonly string[];
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const writable = useActivityWritable();
  const [target, setTarget] = useState<{
    dealId?: string;
    meetingId: string;
    recipients: readonly string[];
  } | null>(null);
  const check = useCurrentMove<NonNullable<typeof target>>(setTarget);
  const selected = target ?? { dealId, meetingId, recipients };
  const [contact, ...alsoTo] = selected.recipients;
  if (contact === undefined || !writable) return null;
  return (
    <>
      <Button
        variant="ai"
        pending={check.isPending}
        onClick={() =>
          check.mutate({
            receipt: recommendation,
            operand: { dealId, meetingId, recipients },
          })
        }
      >
        {t("worklist.verb.draft_follow_up_now")}
      </Button>
      <ErrorLine error={check.error} />
      {target && (
        <ComposeModal
          entityType="contact"
          entityId={contact}
          contactId={contact}
          followUp={{ alsoTo, aboutActivityId: selected.meetingId }}
          kind="email"
          intent={t("contact.composer.intentFollowUp")}
          draftOnOpen
          open
          onClose={() => setTarget(null)}
          onSent={() => {
            if (selected.dealId) {
              void queryClient.invalidateQueries({
                queryKey: ["deal-status", selected.dealId],
              });
            }
            void queryClient.invalidateQueries({ queryKey: ["worklist"] });
          }}
        />
      )}
    </>
  );
}

// A rule wrote the task and pressing this accepts it. No model runs on the
// press, so it is the plain primary, not indigo.
function CreateTaskVerb({
  body,
  pending,
  error,
  onCreate,
}: Readonly<{
  body: Record<string, unknown> | undefined;
  pending: boolean;
  error: unknown;
  onCreate: (body: Record<string, unknown>) => void;
}>) {
  const t = useT();
  const writable = useActivityWritable();
  // No body, no button: a click that sent {} would only be refused, and so
  // would any press from a seat that may not write activities.
  if (!body || !writable) {
    return null;
  }
  return (
    <>
      <Button
        variant="primary"
        pending={pending}
        onClick={() => onCreate(body)}
      >
        <ListChecks aria-hidden />
        {t("deal360.createTask")}
      </Button>
      <ErrorLine error={error} />
    </>
  );
}

/** The follow-up a press just wrote, named, with the way into it. */
function CreatedFollowUp({
  taskId,
  readOnly,
  update,
}: Readonly<{
  taskId: string;
  readOnly: boolean;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <span role="status">{t("deal360.followUpCreated")}</span>
      <Button variant="link" onClick={() => setOpen(true)}>
        {t("deal360.openTask")}
      </Button>
      {open && (
        <TaskDetailModal
          activityId={taskId}
          readOnly={readOnly}
          onClose={() => setOpen(false)}
          update={update}
        />
      )}
    </>
  );
}
