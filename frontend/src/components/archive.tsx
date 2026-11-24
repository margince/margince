// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { undoAction, useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { type RequiredGrant, useGrantRefused } from "../lib/capability";
import { type Departure, fromHere } from "../lib/navigatefrom";
import { problemMessageOf } from "../lib/problem";
import "../design-system/querystates.css";

// The shared archive/disqualify affordance (P-3): a human-direct DELETE that
// soft-archives a contact/company/lead (sets archived_at; leads also
// flip to status=disqualified). Mirrors useUpdateRecord/EditAction (edit.tsx):
// the screen supplies the transport, this stays resource-agnostic.
//
// The restore itself is the screen's: a screen that can take the archive back
// hands `undo`, and the confirmation carries it as its Undo.

export function useArchiveRecord<Archived extends { id: string }, Vars = void>({
  archive,
  invalidate,
  recordKey,
  archivedMessage,
  onDone,
  undo,
}: Readonly<{
  // Takes the caller's own variables, so a form's value reaches the request as
  // an argument rather than through a closure the render decides. A caller with
  // nothing to pass leaves Vars at void and calls mutate() as before.
  archive: (vars: Vars) => Promise<Archived>;
  // A list's key prefix, or the exact keys of the reads the record sits in.
  invalidate: string | readonly QueryKey[];
  recordKey: string;
  // What the reader is told once it is gone, already translated. REQUIRED, and
  // that is the point: the closing dialog was the only thing a reader got and
  // it says nothing about whether the server agreed. A caller that would rather
  // stay silent has to say so by writing the sentence, which is a decision
  // somebody makes rather than one that happens by nobody adding a line.
  //
  // A FUNCTION of the result where the sentence needs a fact only the server
  // knows. Rejecting a company refuses whichever domain was primary at the
  // moment of the write, which is deliberately not the one the page was
  // showing — so a message composed before the call could name the wrong one,
  // and telling somebody the wrong domain is refused is worse than telling
  // them nothing.
  archivedMessage: string | ((archived: Archived) => string);
  // `from` is where the reader confirmed, for a caller that moves them on.
  onDone: (archived: Archived, from: Departure) => void;
  // Takes the archive back. Absent, the confirmation offers no Undo, because
  // an Undo with nothing behind it is worse than none.
  undo?: (archived: Archived) => Promise<void>;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  return useMutation({
    mutationFn: archive,
    onMutate: fromHere,
    onSuccess: (archived, _vars, from) => {
      for (const queryKey of typeof invalidate === "string"
        ? [[invalidate]]
        : invalidate) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: [recordKey, archived.id] });
      onDone(archived, from);
      toast.show(
        typeof archivedMessage === "string"
          ? archivedMessage
          : archivedMessage(archived),
        undo && {
          // The page that drew the verb is gone by now, so a refusal is said
          // here.
          action: undoAction(t("common.undo"), () => {
            undo(archived).catch((error: unknown) =>
              toast.show(problemMessageOf(error, t), { tone: "danger" }),
            );
          }),
        },
      );
    },
  });
}

// The whole per-screen archive affordance in one piece: the danger trigger
// button, a confirm modal (nothing destructive fires without it), and the
// archive choreography above. A screen supplies its label/confirm copy and
// its DELETE transport — nothing else.
export function ArchiveAction<Archived extends { id: string }>({
  label,
  confirmText,
  archive,
  invalidate,
  recordKey,
  archivedMessage,
  onArchived,
  disabledReasonId,
  requires,
  archivable,
  refusedReason,
  undo,
}: Readonly<{
  label: string;
  confirmText: string;
  archive: () => Promise<Archived>;
  invalidate: string | readonly QueryKey[];
  recordKey: string;
  // What the reader is told once it is gone. See `useArchiveRecord`.
  archivedMessage: string;
  onArchived: (from: Departure) => void;
  // Why this action is unavailable, when it is. STATE-4a settles the
  // absent-vs-disabled question by CAUSE: a control blocked by STATE
  // rather than permission — an archived record — stays visible and
  // disabled WITH the reason, because the reason is the information and
  // hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
  // The grant the DELETE needs. Required, so a new caller cannot forget to ask.
  // A seat without it sees the action disabled with the reason, not a dialog
  // that ends in a refusal.
  requires: RequiredGrant;
  // The server's per-row answer, where the record carries one. It decides in
  // place of `requires`, because some rows are archived by their owner or
  // author without the delete grant.
  archivable?: boolean;
  // What a refused reader is told, where the generic role sentence is wrong.
  refusedReason?: string;
  // See `useArchiveRecord`.
  undo?: (archived: Archived) => Promise<void>;
}>) {
  const t = useT();
  const grantRefused = useGrantRefused(requires);
  const refused = archivable === undefined ? grantRefused : !archivable;
  const [confirming, setConfirming] = useState(false);
  const mutation = useArchiveRecord({
    archive,
    invalidate,
    recordKey,
    archivedMessage,
    onDone: (_archived, from) => {
      setConfirming(false);
      onArchived(from);
    },
    undo,
  });

  return (
    <>
      <Button
        variant="danger"
        reasonId={disabledReasonId}
        reason={
          refused ? (refusedReason ?? t("record.actionRefused")) : undefined
        }
        onClick={() => setConfirming(true)}
        data-testid="archive-record"
      >
        {label}
      </Button>
      <ConfirmModal
        open={confirming}
        onClose={() => {
          setConfirming(false);
          mutation.reset();
        }}
        title={label}
        confirmLabel={label}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={mutation.error ? problemMessageOf(mutation.error, t) : null}
      >
        <p>{confirmText}</p>
      </ConfirmModal>
    </>
  );
}
