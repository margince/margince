import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { promotionTriggerLabel } from "../../format/promotion";
import { useT } from "../../i18n";
import { leadWriteKeys } from "../../lib/leadkeys";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordHistory } from "../records/history";

/**
 * What the promotion did, read from the promote audit row it wrote.
 *
 * `outcome` is a closed union with an explicit unknown, not a bare string.
 * The page states "merged into a contact we already knew" or "became a new
 * contact".
 * Reading every non-"merged" value as "created" would turn schema drift, a bad
 * row or a future third outcome into a confident claim about a merge.
 */
type PromotionOutcome = "merged" | "created" | "unknown";

export type PromotionRecord = {
  outcome: PromotionOutcome;
  trigger?: string;
  evidenceNote?: string;
  // The read's own state, since loading and failing are not "created" either.
  // A panel reporting an outcome while its source was in flight would show the
  // wrong one until the request ended, and forever on a 403.
  pending: boolean;
  failed: boolean;
};

/**
 * usePromotionRecord reads the promotion off the lead's audit trail.
 *
 * The outcome, trigger and evidence are not columns on `lead`. The write puts
 * them in the `promote` audit row, the true source. Re-deriving "did this
 * merge?" from today's data would describe the records now, not what happened.
 *
 * Only a promoted lead has a promotion, so the read is disabled on the others.
 */
export function usePromotionRecord(
  id: string,
  promoted: boolean,
): PromotionRecord {
  // One row, asked for by verb: the history read filtered by `action` answers
  // with the promotion itself, wherever in a long trail it sits. A lead is
  // promoted once, so there is no page after the first and nothing to walk.
  const history = useRecordHistory("lead", id, promoted, "promote");
  // `page?.data` for the same reason getNextPageParam needs it. The contract
  // permits a 200 with no body, and this read runs on every promoted lead page.
  const entries = history.data?.pages.flatMap((page) => page?.data ?? []) ?? [];
  const row = entries[0];

  const after = (row?.after ?? {}) as Record<string, unknown>;
  const str = (key: string) =>
    typeof after[key] === "string" ? (after[key] as string) : undefined;
  const recorded = str("dedupe_outcome");
  return {
    outcome:
      recorded === "merged" || recorded === "created" ? recorded : "unknown",
    trigger: str("trigger"),
    evidenceNote: str("evidence_note"),
    // A read in flight is pending: reporting "we cannot tell" before the answer
    // arrives is as false as reporting "created". A failed read is never
    // pending. The panel checks pending first, so both true hides the error.
    pending: promoted && !history.isError && history.isPending,
    failed: promoted && history.isError,
  };
}

/**
 * DemoteAction reverses a promotion, as ADR-0008 requires every promotion to be
 * reversible. It lives on the one page that is a fact about the promotion. A
 * reason is required and recorded: an unexplained undo looks like a mistake.
 */
export function DemoteAction({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const demote = useMutation({
    mutationFn: async (input: { id: string; reason: string }) => {
      return unwrap(
        await api.POST("/leads/{id}/demote", {
          params: { path: { id: input.id } },
          body: { reason: input.reason },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const key of leadWriteKeys(id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      setOpen(false);
      setReason("");
    },
  });
  const close = () => {
    setOpen(false);
    demote.reset();
  };
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lead.demote")}</Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("lead.demoteDialog")}
        confirmLabel={t("lead.demoteConfirm")}
        confirmReason={
          reason.trim() === "" ? t("lead.demoteReasonRequired") : undefined
        }
        onConfirm={() => demote.mutate({ id, reason: reason.trim() })}
        pending={demote.isPending}
        error={demote.isError ? problemMessageOf(demote.error, t) : undefined}
      >
        <p className="t-body">{t("lead.demoteExplain")}</p>
        <Field label={t("lead.demoteReason")} required>
          {(control) => (
            <Textarea
              {...control}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}

/**
 * What the qualification did, in one line for the "Qualified as" card. It says
 * whether it merged into a contact we already knew or created one, then the
 * trigger and evidence it recorded.
 *
 * Four outcome states, not two. The contact link is a fact the lead row
 * carries, so it renders either way; only the outcome waits on the audit read.
 */
export function promotionSummary(
  promotion: PromotionRecord,
  t: ReturnType<typeof useT>,
): string {
  const triggerLabel = promotionTriggerLabel(promotion.trigger);
  const outcome = () => {
    if (promotion.pending) {
      return t("lead.promotedOutcomePending");
    }
    if (promotion.failed) {
      return t("lead.promotedOutcomeUnavailable");
    }
    switch (promotion.outcome) {
      case "merged":
        return t("lead.promotedMerged");
      case "created":
        return t("lead.promotedCreated");
      // The audit row is missing, unreadable, or names an outcome this build
      // does not know. Saying so is correct; picking one would be a claim about
      // a merge nobody recorded.
      case "unknown":
        return t("lead.promotedOutcomeUnavailable");
    }
  };
  return [
    outcome(),
    triggerLabel && `${t("lead.promotedTrigger")} ${t(triggerLabel)}`,
    promotion.evidenceNote &&
      `${t("lead.promotedEvidence")} ${promotion.evidenceNote}`,
  ]
    .filter(Boolean)
    .join(" ");
}
