// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";

// The shared archive/disqualify affordance (P-3): a human-direct DELETE that
// soft-archives a contact/company/lead (sets archived_at; leads also
// flip to status=disqualified). There is NO restore endpoint in the
// contract, so this hook and action are archive-only — never wire a restore
// control against them. Mirrors useUpdateRecord/EditAction (edit.tsx): the
// screen supplies the transport, this stays resource-agnostic.

export function useArchiveRecord<Archived extends { id: string }, Vars = void>({
  archive,
  invalidate,
  recordKey,
  archivedMessage,
  onDone,
}: Readonly<{
  // Takes the caller's own variables, so a form's value reaches the request as
  // an argument rather than through a closure the render decides. A caller with
  // nothing to pass leaves Vars at void and calls mutate() as before.
  archive: (vars: Vars) => Promise<Archived>;
  invalidate: string;
  recordKey: string;
  // What the reader is told once it is gone, already translated. REQUIRED, and
  // that is the point: an archive is destructive and has no restore endpoint
  // behind it, so the closing dialog was the only thing a reader got and it
  // says nothing about whether the server agreed. A caller that would rather
  // stay silent has to say so by writing the sentence, which is a decision
  // somebody makes rather than one that happens by nobody adding a line.
  //
  // A FUNCTION of the result where the sentence needs a fact only the server
  // knows. Rejecting a company refuses whichever domain was primary at the
  // moment of the write, which is deliberately not the one the page was
  // showing — so a message composed before the call could name the wrong one,
  // and telling somebody the wrong domain is refused is worse than telling
  // them nothing.
  //
  // No `action` here for the same reason the comment at the top of this file
  // gives: the contract has no restore endpoint, and an Undo with nothing
  // behind it is worse than none.
  archivedMessage: string | ((archived: Archived) => string);
  onDone: (archived: Archived) => void;
}>) {
  const queryClient = useQueryClient();
  const toast = useToast();
  return useMutation({
    mutationFn: archive,
    onSuccess: (archived) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      queryClient.invalidateQueries({ queryKey: [recordKey, archived.id] });
      onDone(archived);
      toast.show(
        typeof archivedMessage === "string"
          ? archivedMessage
          : archivedMessage(archived),
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
}: Readonly<{
  label: string;
  confirmText: string;
  archive: () => Promise<Archived>;
  invalidate: string;
  recordKey: string;
  // What the reader is told once it is gone. See `useArchiveRecord`.
  archivedMessage: string;
  onArchived: () => void;
  // Why this action is unavailable, when it is. STATE-4a settles the
  // absent-vs-disabled question by CAUSE: a control blocked by STATE
  // rather than permission — an archived record — stays visible and
  // disabled WITH the reason, because the reason is the information and
  // hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const headingId = useId();
  const [confirming, setConfirming] = useState(false);
  const mutation = useArchiveRecord({
    archive,
    invalidate,
    recordKey,
    archivedMessage,
    onDone: () => {
      setConfirming(false);
      onArchived();
    },
  });

  return (
    <>
      <Button
        small
        variant="danger"
        reasonId={disabledReasonId}
        onClick={() => setConfirming(true)}
        data-testid="archive-record"
      >
        {label}
      </Button>
      <Modal
        open={confirming}
        onClose={() => setConfirming(false)}
        labelledBy={headingId}
      >
        <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
          {label}
        </h2>
        <p style={{ marginBottom: 16 }}>{confirmText}</p>
        {mutation.isError && (
          // role="alert" so a refused archive is announced: the dialog stays
          // open either way, and without this the only difference between "it
          // failed" and "it is still working" is a line of red text.
          <p
            className="t-caption"
            role="alert"
            style={{ color: "var(--dangerText)" }}
          >
            {problemMessageOf(mutation.error, t)}
          </p>
        )}
        <div className="actions">
          <Button
            small
            onClick={() => setConfirming(false)}
            disabled={mutation.isPending}
          >
            {t("create.cancel")}
          </Button>
          <Button
            small
            variant="danger"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
            data-testid="archive-confirm"
          >
            {label}
          </Button>
        </div>
      </Modal>
    </>
  );
}
