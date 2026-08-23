// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { Button, Field, Modal, Textarea } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  isVersionSkew,
  ProblemError,
  problemMessageOf,
  throwProblem,
} from "./common";

// Moving one commission entry through the ledger's lifecycle.
//
// MARGINCE DOES NOT PAY ANYBODY. Paying a partner happens in a finance system
// and always will; what this records is what that system already did. The copy
// says so at the point of the decision rather than in a doc nobody opens,
// because "Mark as paid" beside a money figure reads like a payment button and
// exactly once is enough for somebody to believe it.
//
// The three transitions are the store's, not this file's opinion of them
// (commissions/decide.go, legalTransitions): accrued may be approved, approved
// may be paid, and anything still live may be reversed. Offering a fourth here
// would put a control on screen the server refuses.

type CommissionEntry = components["schemas"]["CommissionEntry"];
type Decision = "approve" | "pay" | "void";

// What each decision is called, what it says before you commit, and the
// confirmation it leaves behind.
const DECISION_COPY: Record<
  Decision,
  { label: MessageKey; confirm: MessageKey; done: MessageKey }
> = {
  approve: {
    label: "commission.decide.approve",
    confirm: "commission.decide.approveConfirm",
    done: "commission.decide.approved",
  },
  pay: {
    label: "commission.decide.pay",
    confirm: "commission.decide.payConfirm",
    done: "commission.decide.paid",
  },
  void: {
    label: "commission.decide.void",
    confirm: "commission.decide.voidConfirm",
    done: "commission.decide.voided",
  },
};

/**
 * decisionsFor names what this entry's CURRENT state admits.
 *
 * It mirrors `legalTransitions` in the commissions store. A control the server
 * would refuse is worse than a missing one: the reader presses it, gets a 422,
 * and learns the rule from a refusal instead of from the screen.
 */
export function decisionsFor(status: CommissionEntry["status"]): Decision[] {
  switch (status) {
    case "accrued":
      return ["approve", "void"];
    case "approved":
      return ["pay", "void"];
    // A paid entry can still be reversed — money going out and coming back is
    // a thing that happens, and the reversal row is the record of it.
    case "paid":
      return ["void"];
    // Void is terminal. Nothing follows it, which is what makes the ledger
    // readable a year later.
    default:
      return [];
  }
}

async function decide(
  entry: CommissionEntry,
  decision: Decision,
  reason: string,
): Promise<CommissionEntry> {
  const { data, error } = await api.POST("/commissions/{id}/decide", {
    params: {
      path: { id: entry.id },
      // The entry carries its own version, so two people deciding the same
      // row at once get a 409 rather than the second one silently winning.
      ...ifMatch(entry.version ?? 0),
    },
    body: {
      decision,
      // Only void takes a reason, and the server requires it. Sending an
      // empty string on the other two would be a value nobody asked for.
      ...(decision === "void" ? { reason } : {}),
    },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

/**
 * CommissionDecision is one verb on one ledger row: the trigger, the confirm
 * step a money decision deserves, and the write.
 *
 * Every decision confirms. None of these is reversible by pressing the same
 * button again — approve and pay move forward only, and void is terminal — so
 * there is no state where a misclick costs nothing.
 */
export function CommissionDecision({
  entry,
  decision,
  organizationId,
}: Readonly<{
  entry: CommissionEntry;
  decision: Decision;
  organizationId: string;
}>) {
  const t = useT();
  const { show: showToast } = useToast();
  const queryClient = useQueryClient();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  // Where focus goes when the dialog closes. On success the row re-renders
  // into its new status and the verb that opened this is gone, so without a
  // named target a keyboard reader is dropped to the top of the document.
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [reason, setReason] = useState("");
  // Only shown after a submit is attempted: a required-field message that
  // appears while the reader is still typing is telling them off for not
  // having finished.
  const [showReasonFault, setShowReasonFault] = useState(false);

  const copy = DECISION_COPY[decision];
  const needsReason = decision === "void";
  const reasonGiven = reason.trim() !== "";

  const mutation = useMutation({
    mutationFn: () => decide(entry, decision, reason.trim()),
    onError: (err) => {
      // A 409 means somebody else moved this entry while the dialog was open,
      // so the version in hand is stale and pressing again resends it. Refetch
      // so the retry carries the version the server now holds, and say what
      // happened rather than showing the server's own "apply commission
      // decision: version skew".
      if (err instanceof ProblemError && isVersionSkew(err.problem)) {
        queryClient.invalidateQueries({
          queryKey: ["partner-commissions", organizationId],
        });
      }
    },
    onSuccess: () => {
      // One key, because the outstanding figure above the ledger is derived
      // from these same rows rather than fetched separately — invalidating a
      // second key nothing reads would be a line that looks like caution and
      // does nothing.
      queryClient.invalidateQueries({
        queryKey: ["partner-commissions", organizationId],
      });
      setOpen(false);
      setReason("");
      setShowReasonFault(false);
      showToast(t(copy.done));
    },
  });

  function submit() {
    if (needsReason && !reasonGiven) {
      setShowReasonFault(true);
      return;
    }
    mutation.mutate();
  }

  return (
    <>
      <Button
        ref={triggerRef}
        small
        variant={decision === "void" ? "danger" : "primary"}
        onClick={() => setOpen(true)}
        data-testid={`commission-${decision}`}
      >
        {t(copy.label)}
      </Button>
      <Modal
        open={open}
        // A write in flight refuses the dismissal too, not only the Cancel
        // button. Escape and the backdrop reach past a disabled button, and
        // closing here would hide the dialog without cancelling the POST —
        // leaving the row's other verb clickable and two conflicting
        // decisions racing. If-Match stops both committing; which one wins
        // would be luck.
        onClose={() => {
          if (!mutation.isPending) {
            setOpen(false);
          }
        }}
        labelledBy={headingId}
        // On success the row re-renders into its new status and the verb that
        // opened this is gone, so a keyboard reader would be dropped to the
        // top of the document without a named target.
        returnFocusTo={() => triggerRef.current}
      >
        <h2
          id={headingId}
          className="t-h2"
          style={{ marginBottom: "var(--space-3)" }}
        >
          {t(copy.label)}
        </h2>
        <p style={{ marginBottom: "var(--space-4)" }}>{t(copy.confirm)}</p>
        {needsReason && (
          <div style={{ marginBottom: "var(--space-4)" }}>
            <Field
              label={t("commission.decide.reasonLabel")}
              required
              error={
                showReasonFault && !reasonGiven
                  ? t("commission.decide.reasonRequired")
                  : undefined
              }
            >
              {(control) => (
                <Textarea
                  {...control}
                  value={reason}
                  rows={3}
                  onChange={(e) => setReason(e.target.value)}
                  data-testid="commission-void-reason"
                />
              )}
            </Field>
          </div>
        )}
        {mutation.isError && (
          // role="alert" so a refused decision is announced: the dialog stays
          // open either way, and without this the only difference between "it
          // failed" and "it is still working" is a line of red text.
          <p
            className="t-caption"
            role="alert"
            style={{ color: "var(--danger)" }}
          >
            {mutation.error instanceof ProblemError &&
            isVersionSkew(mutation.error.problem)
              ? t("edit.versionSkew")
              : problemMessageOf(mutation.error, t)}
          </p>
        )}
        <div className="actions">
          <Button
            small
            onClick={() => setOpen(false)}
            disabled={mutation.isPending}
          >
            {t("create.cancel")}
          </Button>
          <Button
            small
            variant={decision === "void" ? "danger" : "primary"}
            onClick={submit}
            pending={mutation.isPending}
            data-testid={`commission-${decision}-confirm`}
          >
            {t(copy.label)}
          </Button>
        </div>
      </Modal>
    </>
  );
}
