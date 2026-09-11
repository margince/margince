// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Performing the next step a producer already decided.
//
// TWO surfaces draw the same move. The deal status card names it beside its
// reason and evidence; the worklist row names it on a queue line. They differ
// in what they say ABOUT the move and not in what pressing it does, so the
// button lives here and both mount it — a second implementation would be two
// answers to "what does Create task do", and they would drift until a rep
// pressed the same words on two screens and got different results.
//
// Typed on the pair both wire shapes carry rather than on either of them.
// DealStatusCardMove requires a reason and evidence, WorklistMove carries
// neither, and nothing here reads either one: the verb needs its action and its
// operand and nothing else.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ListChecks, Sparkles } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { Button } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { ContactMeetingBrief } from "./meetingbrief";
import { useOpenEmail } from "./openemail";
import { TaskDetailModal, useTaskUpdate } from "./taskactions";

// What a move must carry to be performed. Both producers' shapes satisfy it.
export type PerformableMove = {
  action: string;
  arguments?: Record<string, unknown>;
};

// activityIdOf reads the one operand the meeting-brief verb takes. The
// arguments object is typed open on the wire; a missing id renders no button
// rather than a button that would 404.
export function activityIdOf(move: PerformableMove): string | null {
  const raw = move.arguments?.activity_id;
  return typeof raw === "string" && raw !== "" ? raw : null;
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
    default:
      return false;
  }
}

export function MoveButton({
  dealId,
  move,
}: Readonly<{
  // The deal whose card to refresh after the verb writes, where the caller has
  // one. OPTIONAL because the worklist does not: a queue row's move may belong
  // to a deal, a contact or nothing at all, and passing an empty string would
  // invalidate a key naming no deal — a call that does nothing, spelled as one
  // that does something.
  dealId?: string;
  move: PerformableMove;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [briefOpen, setBriefOpen] = useState(false);
  // This button's own drawer. Every other host of the meeting brief already
  // mounts one for its timeline; this one is a lone control on the deal card,
  // so a message cited in the brief it opens has nowhere else to go.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useRecordZone();
  const [taskOpen, setTaskOpen] = useState(false);
  const canUpdateTask = useCan("activity", "update");
  const taskUpdate = useTaskUpdate([
    ...(dealId ? [["deal-status", dealId]] : []),
    ["tasks"],
    ["worklist"],
  ]);
  const activityId = activityIdOf(move);
  const createTask = useMutation({
    mutationKey: ["deal-status-create-task"],
    // The arguments ARE the task body the server prepared; the click sends
    // them as they came rather than re-deriving them from render state.
    mutationFn: async (body: Record<string, unknown>) => {
      const { data, error } = await api.POST("/tasks", {
        body: body as components["schemas"]["CreateTaskRequest"],
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
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
    },
  });

  const taskBody = move.arguments;
  switch (move.action) {
    case "open_task":
      if (!activityId) return null;
      return (
        <>
          <Button small onClick={() => setTaskOpen(true)}>
            {t("deal360.openTask")}
          </Button>
          {taskOpen && (
            <TaskDetailModal
              activityId={activityId}
              readOnly={!canUpdateTask}
              onClose={() => setTaskOpen(false)}
              update={taskUpdate}
            />
          )}
        </>
      );
    case "create_task":
      // No body, no button: a click that sent {} would only be refused.
      if (!taskBody) {
        return null;
      }
      return (
        <>
          <Button
            variant="primary"
            small
            pending={createTask.isPending}
            onClick={() => createTask.mutate(taskBody)}
          >
            <ListChecks aria-hidden />
            {t("deal360.createTask")}
          </Button>
          {createTask.isError ? (
            <p className="t-caption t-danger">
              {problemMessageOf(createTask.error, t)}
            </p>
          ) : null}
        </>
      );
    case "draft_email":
      // No button here. Writing to the buyer is the email box's job, in the
      // right-hand column under the Deal Room — one place a rep goes to send
      // mail, whether or not this move happens to rank first. Deal360 still
      // says WHY the mail is the move; it just does not carry a second door
      // to the same composer.
      return null;
    case "open_meeting_brief":
      if (!activityId) {
        return null;
      }
      return (
        <>
          <Button variant="primary" small onClick={() => setBriefOpen(true)}>
            <Sparkles aria-hidden />
            {t("deal360.openBrief")}
          </Button>
          <ContactMeetingBrief
            activityId={activityId}
            open={briefOpen}
            onClose={() => setBriefOpen(false)}
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
