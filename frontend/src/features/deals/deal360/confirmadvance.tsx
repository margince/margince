import { type FormEvent, useState } from "react";
import type { components } from "../../../api/schema";
import { Field, TextInput } from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { Select } from "../../../design-system/select";
import { useT } from "../../../i18n";
import { useAgentTierMap, verbTier } from "../../../lib/autonomy";
import { problemFieldErrorsOf } from "../../../lib/problem";
import {
  WON_REASON_LABELS,
  WON_REASONS,
  type WonReason,
} from "../../../lib/winreason";
import { closeReason, isSavedDeal } from "./closereviewoffer";
import { CloseTasksQuestion, useCloseTasks } from "./closetasks";
import { useWinQuestion } from "./winevidence";
import "./deal360.css";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// Narrows the Select's plain string back to the vocabulary. The control is
// built from WON_REASONS, so this never rejects in practice — but a cast would
// make that an assumption instead of a check, and the value goes on to be
// stored as an assertion about how a deal closed.
function asWonReason(value: string): WonReason | "" {
  const known = WON_REASONS.find((reason) => reason === value);
  return known ?? "";
}

/**
 * Whether a detail carries any visible character, matching the rule the server
 * applies (`saysSomething` in win_evidence.go).
 *
 * `trim()` alone is not the same test. A zero-width space is not whitespace to
 * either language, so a detail of "​" would pass a trim check here, enable
 * Confirm, and then be refused by the server — the reader having explained
 * precisely nothing, which is the state the vocabulary exists to prevent.
 */
function saysSomething(text: string): boolean {
  return /\P{White_Space}/u.test(text.replace(/\p{Cf}/gu, ""));
}

// The one member that explains nothing on its own, so the server demands a
// detail after it (`WonReasonDetailRequiredError`).
const WON_REASON_NEEDING_DETAIL: WonReason = "other";

// The server's refusal when a win names neither a contract nor a reason. The
// dialog keys on THIS rather than on the 422 status: an advance can be refused
// for reasons that have nothing to do with evidence, and asking "how was it
// won?" after a version conflict would be nonsense.
const WIN_EVIDENCE_REQUIRED = "win_evidence_required";

export type PendingAdvance = {
  dealId: string;
  // Carried through the confirm rather than looked up when it closes: the write
  // pins the deal as it stood on the board the reader dropped it on, so a stage
  // change made while the dialog was open fails loud instead of being erased.
  version: number | undefined;
  toStage: Stage;
};

export type AdvanceInput = {
  dealId: string;
  version: number | undefined;
  toStage: Stage;
  lostReason?: string;
  // Why this win has no signed contract behind it. Absent on the ordinary win,
  // where the contract IS the answer — the server distinguishes the two, and
  // that distinction is what makes "how many won deals have no paper" a
  // question reports can answer.
  wonWithoutContractReason?: WonReason;
  wonWithoutContractDetail?: string;
};

/**
 * The 🟡 confirm a terminal advance goes through (AC-deal-6), wherever the
 * advance was asked for — the board's drag or the record page's stepper.
 *
 * Closing a deal is the one stage move that cannot be undone by moving it
 * back, so the question is asked in ONE place: a second copy of this dialog is
 * how the two surfaces would end up disagreeing about whether a lost deal
 * needs a reason.
 */
export function ConfirmAdvanceModal({
  pending,
  onClose,
  onConfirm,
  onClosed,
}: Readonly<{
  pending: PendingAdvance | null;
  onClose: () => void;
  // Told about a close that LANDED, so the surface owning this dialog can
  // offer the outcome review while the deal is still in mind. Called with the
  // deal the server handed back and the reason this dialog required, and only
  // on a terminal move that actually succeeded.
  onClosed?: (deal: Deal, reason: string) => void;
  // Settles with the SAVED DEAL, which the review offered next needs. A
  // rejection is read as the outcome, not thrown: the caller's own error surface
  // reports it, and thrown from a click handler it would be unhandled.
  onConfirm: (input: AdvanceInput) => Promise<unknown>;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const [lostReason, setLostReason] = useState("");
  const [wonReason, setWonReason] = useState<WonReason | "">("");
  const [wonDetail, setWonDetail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  // Read only while a close is being asked about; every move this dialog
  // confirms is to a won or lost stage.
  const closeTasks = useCloseTasks(pending?.dealId ?? null);
  // The deal this dialog has been told has no contract behind it, pinned to the
  // exact attempt that was refused.
  //
  // Read off the shared mutation's `error` instead, the refusal outlives the
  // deal that earned it: cancel here, open Won on a DIFFERENT deal, and the
  // reason panel greets a deal that may well have a contract — and the server
  // takes a stated reason at its word without looking for one, so that deal is
  // recorded as won-without-paper when it was not. That falsifies the exact
  // count the reason vocabulary exists to make truthful.
  const [refusedDealId, setRefusedDealId] = useState<string | null>(null);
  // Still drawn while it animates out after `pending` clears.
  const [lastAsked, setLastAsked] = useState(pending);
  if (pending !== null && pending !== lastAsked) {
    setLastAsked(pending);
  }
  const shown = pending ?? lastAsked;

  // EVERY way out of this dialog clears what was typed — the buttons, Escape,
  // and the backdrop alike. The component stays mounted between openings, so a
  // reason typed and then abandoned would otherwise still be sitting there the
  // next time a deal is closed, and it would describe a different deal.
  const dismiss = () => {
    setSubmitting(false);
    setLostReason("");
    setWonReason("");
    setWonDetail("");
    setRefusedDealId(null);
    closeTasks.reset();
    onClose();
  };

  const needsLostReason = shown?.toStage.semantic === "lost";
  // Only a win with no signed contract is asked how it happened. A question
  // every rep must answer gets the same lie from every rep.
  const winQuestion = useWinQuestion(pending, refusedDealId, submitting);
  const needsWonReason = winQuestion.needed;
  const detailMissing =
    wonReason === WON_REASON_NEEDING_DETAIL && !saysSomething(wonDetail);
  // Said beside Confirm rather than left to a grey button: the reader is told
  // which answer the close still needs.
  const unanswered = (() => {
    if (needsLostReason && lostReason.trim() === "") {
      return t("deals.lostReasonNeeded");
    }
    if (needsWonReason && wonReason === "") {
      return t("deals.winReasonNeeded");
    }
    return needsWonReason && detailMissing
      ? t("deals.winDetailNeeded")
      : undefined;
  })();
  const refusal =
    unanswered ??
    (winQuestion.checking ? t("deals.winEvidenceChecking") : undefined) ??
    closeTasks.refusal;

  const confirm = async (asked: PendingAdvance) => {
    setSubmitting(true);
    // Captured BEFORE the await: `dismiss()` clears what was typed, and the
    // review starts from the words this reader actually wrote for THIS close.
    const tasksToComplete = closeTasks.chosen;
    const withoutPaper =
      needsWonReason && (await winQuestion.stillBare(asked.dealId));
    const why = closeReason({
      wonAsked: withoutPaper,
      lost: lostReason,
      won: wonReason,
      detail: wonDetail,
      t,
    });
    const result = await onConfirm({
      dealId: asked.dealId,
      version: asked.version,
      toStage: asked.toStage,
      lostReason: lostReason.trim() || undefined,
      ...wonAnswer(withoutPaper, wonReason, wonDetail),
    }).catch((error: unknown) => error);
    setSubmitting(false);
    // Only the missing-evidence refusal stays open: the reader can answer it
    // here, and any other error renders on the screen behind this dialog.
    if (!isSavedDeal(result)) {
      if (winEvidenceRefused(result)) {
        setRefusedDealId(asked.dealId);
        return;
      }
    } else {
      onClosed?.(result, why);
      // Only after the close landed: a refused close completes nothing.
      void closeTasks.complete(tasksToComplete);
    }
    dismiss();
  };

  // Enter in a reason field closes the deal only when Confirm would.
  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    if (pending && refusal === undefined && !submitting) {
      void confirm(pending);
    }
  };

  return (
    <ConfirmModal
      open={pending !== null}
      onClose={dismiss}
      title={
        shown ? t("deals.confirmAdvance", { stage: shown.toStage.name }) : ""
      }
      tier={verbTier("progress_deal", tierMap)}
      confirmLabel={t("deals.confirm")}
      confirmReason={refusal}
      pending={submitting}
      onConfirm={submit}
    >
      {shown && (
        <>
          <p className="t-caption">
            {t("deals.confirmTerminal", { status: shown.toStage.semantic })}
          </p>
          {(needsLostReason || needsWonReason) && (
            <form className="form-stack" onSubmit={submit}>
              {needsLostReason && (
                <Field label={t("deals.lostReason")}>
                  {(control) => (
                    <TextInput
                      {...control}
                      value={lostReason}
                      onChange={(event) => setLostReason(event.target.value)}
                    />
                  )}
                </Field>
              )}
              {needsWonReason && (
                <WonReasonFields
                  reason={wonReason}
                  detail={wonDetail}
                  onReason={(next) => {
                    setWonReason(next);
                    // A detail kept behind a hidden field is one the reader
                    // cannot correct.
                    if (next !== WON_REASON_NEEDING_DETAIL) {
                      setWonDetail("");
                    }
                  }}
                  onDetail={setWonDetail}
                />
              )}
            </form>
          )}
          <CloseTasksQuestion state={closeTasks} />
        </>
      )}
    </ConfirmModal>
  );
}

/**
 * The won-without-contract answer as the advance should carry it, or nothing.
 *
 * The detail rides ONLY with the reason that needs one. Sent alongside any
 * other reason it would be stored anyway — the server writes both columns as
 * given — so a reader who typed a detail under "Something else" and then chose
 * "On a purchase order" would leave text on the deal, and in its audit trail,
 * that they had every reason to believe they had discarded when the field
 * disappeared.
 */
function wonAnswer(active: boolean, reason: WonReason | "", detail: string) {
  if (!active || reason === "") {
    return {};
  }
  const explained = reason === WON_REASON_NEEDING_DETAIL;
  return {
    wonWithoutContractReason: reason,
    wonWithoutContractDetail: explained ? detail.trim() : undefined,
  };
}

// The advance failure the screen behind the dialog shows. An open dialog
// answers the missing-evidence refusal itself; once closed, the screen says it.
export function advanceErrorBehind(
  error: unknown,
  dialogOpen: boolean,
): unknown {
  return dialogOpen && winEvidenceRefused(error) ? null : error;
}

// Whether a failed advance is the server asking how a contract-less deal was
// won. Keyed on the field code, not the 422: an advance is refused for several
// reasons, and only this one has an answer the reader can give here.
function winEvidenceRefused(error: unknown): boolean {
  return problemFieldErrorsOf(error).some(
    (fault) => fault.code === WIN_EVIDENCE_REQUIRED,
  );
}

// The reason a deal was won with no paper behind it: a closed vocabulary, plus
// the free-text detail the one open-ended member needs.
function WonReasonFields({
  reason,
  detail,
  onReason,
  onDetail,
}: Readonly<{
  reason: WonReason | "";
  detail: string;
  onReason: (value: WonReason | "") => void;
  onDetail: (value: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <p>{t("deals.winNoEvidence")}</p>
      <Field label={t("deals.winReason")}>
        {(control) => (
          <Select
            {...control}
            placeholder={t("deals.winReasonPick")}
            value={reason}
            onChange={(value) => onReason(asWonReason(value))}
            options={WON_REASONS.map((option) => ({
              value: option,
              label: t(WON_REASON_LABELS[option]),
            }))}
          />
        )}
      </Field>
      {reason === WON_REASON_NEEDING_DETAIL && (
        <Field label={t("deals.winReasonDetail")}>
          {(control) => (
            <TextInput
              {...control}
              value={detail}
              onChange={(event) => onDetail(event.target.value)}
            />
          )}
        </Field>
      )}
    </>
  );
}
