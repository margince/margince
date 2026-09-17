import { useState } from "react";
import type { components } from "../../api/schema";
import { useAgentTierMap, verbTier } from "../../app/autonomy";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { AutonomyDot } from "../../design-system/trust";
import { useT } from "../../i18n";
import { problemFieldErrorsOf } from "../common";
import { WON_REASON_LABELS, WON_REASONS, type WonReason } from "../winreason";
import { closeReason, isSavedDeal } from "./closereviewoffer";
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
  // Resolves when the advance settles, so this dialog acts on the outcome of
  // THIS attempt. It returns the SAVED DEAL on success and the error on
  // failure rather than throwing: the caller's own error surface still reports
  // the failure, and a rejection here would be an unhandled one in an event
  // handler.
  onConfirm: (input: AdvanceInput) => Promise<Deal | unknown>;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const [lostReason, setLostReason] = useState("");
  const [wonReason, setWonReason] = useState<WonReason | "">("");
  const [wonDetail, setWonDetail] = useState("");
  const [submitting, setSubmitting] = useState(false);
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

  // EVERY way out of this dialog clears what was typed — the buttons, Escape,
  // and the backdrop alike. The component stays mounted between openings, so a
  // reason typed and then abandoned would otherwise still be sitting there the
  // next time a deal is closed, and it would describe a different deal.
  const dismiss = () => {
    setLostReason("");
    setWonReason("");
    setWonDetail("");
    setRefusedDealId(null);
    onClose();
  };

  const needsLostReason = pending?.toStage.semantic === "lost";
  // The reason panel appears only once the server has asked for it, and only
  // for the deal it asked about. A win with a signed contract is one click,
  // exactly as before: making every rep justify a win the paperwork already
  // explains is how a required field becomes a field everyone fills with the
  // same lie.
  const needsWonReason =
    pending?.toStage.semantic === "won" && refusedDealId === pending.dealId;
  const detailMissing =
    wonReason === WON_REASON_NEEDING_DETAIL && !saysSomething(wonDetail);

  return (
    <Modal open={pending !== null} onClose={dismiss} labelledBy="advance-title">
      {pending && (
        <>
          <p className="t-sub" id="advance-title">
            <AutonomyDot tier={verbTier("progress_deal", tierMap)} />{" "}
            {t("deals.confirmAdvance", { stage: pending.toStage.name })}
          </p>
          <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
            {t("deals.confirmTerminal", { status: pending.toStage.semantic })}
          </p>
          {(needsLostReason || needsWonReason) && (
            <div className="advance-reasons">
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
                    // The detail belongs to "Something else" alone. Kept across
                    // a change of reason it would sit invisibly behind a field
                    // the reader can no longer see, which is not a state they
                    // can correct.
                    if (next !== WON_REASON_NEEDING_DETAIL) {
                      setWonDetail("");
                    }
                  }}
                  onDetail={setWonDetail}
                />
              )}
            </div>
          )}
          <div className="actions">
            <Button onClick={dismiss}>{t("deals.cancel")}</Button>
            <Button
              variant="primary"
              disabled={
                submitting ||
                (needsLostReason && lostReason.trim() === "") ||
                (needsWonReason && (wonReason === "" || detailMissing))
              }
              onClick={async () => {
                setSubmitting(true);
                // Captured BEFORE the await: `dismiss()` clears what was
                // typed, and the review starts from the words this reader
                // actually wrote for THIS close.
                const why = closeReason({
                  wonAsked: needsWonReason,
                  lost: lostReason,
                  won: wonReason,
                  detail: wonDetail,
                  t,
                });
                const result = await onConfirm({
                  dealId: pending.dealId,
                  version: pending.version,
                  toStage: pending.toStage,
                  lostReason: lostReason.trim() || undefined,
                  ...wonAnswer(needsWonReason, wonReason, wonDetail),
                });
                setSubmitting(false);
                // ONE refusal keeps this dialog open: the server saying this
                // win names no evidence, because the answer to that is a field
                // the reader can fill in right here. Every other outcome closes
                // it — a success has nothing left to ask, and a 403 or a 409
                // has no answer this dialog can offer, so holding it open would
                // trap the reader behind a modal whose error text renders on
                // the screen underneath it.
                if (!isSavedDeal(result)) {
                  if (winEvidenceRefused(result)) {
                    setRefusedDealId(pending.dealId);
                    return;
                  }
                } else {
                  // The close LANDED: offer the review, with this reason
                  // already filled in.
                  onClosed?.(result, why);
                }
                dismiss();
              }}
            >
              {t("deals.confirm")}
            </Button>
          </div>
        </>
      )}
    </Modal>
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
