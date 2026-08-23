// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { Button, Field, Modal, Textarea } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";

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
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["partner-commissions", organizationId],
      });
      // The liability figure on the same page is derived from these rows, so
      // it goes stale the moment one moves.
      queryClient.invalidateQueries({
        queryKey: ["commission-summary", organizationId],
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
        small
        variant={decision === "void" ? "danger" : "primary"}
        onClick={() => setOpen(true)}
        data-testid={`commission-${decision}`}
      >
        {t(copy.label)}
      </Button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy={headingId}>
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
            {problemMessageOf(mutation.error, t)}
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
