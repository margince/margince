// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useRef, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { ApprovalBundleReview } from "./worklist.bundle";
import {
  useApproval,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";

// Whether this row is a decision a contact answers HERE.
//
// The queue holds no authority of its own — the card below is the same one the
// record page draws, posting to the same endpoint. What the queue adds is that
// the decision is answerable where it was ranked, instead of sending a reader
// to a second screen to do what the row already described.
export function decidable(item: WorklistItem): boolean {
  return item.actions.includes("decide") && item.source === "approval";
}

// The decision itself, fetched whole because a row cannot carry it — and
// answered in a DRAWER rather than in the row.
//
// It used to render inline, and that is what made the queue unusable on a
// phone. The card carries evidence, a draft and three answers; measured at
// 390x844 it stood 440px tall inside a row whose ceiling is 208, which pushed
// the first primary action of the whole page to 920px down an 844px screen.
// The reader had to scroll past one decision to reach the work.
//
// The row keeps the decision's SUMMARY and one button. The drawer holds the
// card — the same ApprovalRow the record page draws, posting to the same
// endpoint — so the queue still adds no authority of its own. What it adds is
// that the decision is answerable where it was ranked.
//
// Held by: AC-WORKLIST-SDR-01 and AC-WORKLIST-SDR-07 (frontend/e2e/ac.spec.ts),
// which measure the closed row and the first action against the phone fold.
export function RowDecision({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  // Fetched only once the reader asks. A queue of decisions would otherwise
  // fire one read per row on arrival to fill cards nobody has opened, and the
  // row above needs none of it to draw its button.
  const approval = useApproval(item.id, open);
  const usable = approval.data?.kind ? approval.data : undefined;
  return (
    <div className="worklist-row-decision">
      <Button
        ref={opener}
        variant="primary"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
      >
        {t("worklist.verb.decide")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        placement="right"
        size="wide"
        returnFocusTo={() => opener.current}
      >
        <div className="drawer-head worklist-decision-head">
          <h2 id={titleId} className="t-h2">
            {t("worklist.decision.title")}
          </h2>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            {t("common.close")}
          </Button>
        </div>
        <div className="drawer-body">
          {usable?.bundle_id ? (
            <ApprovalBundleReview approval={usable} />
          ) : usable ? (
            <ApprovalRow
              approval={usable}
              extraInvalidateKeys={[worklistKey]}
              onAlreadyDecided={() => setOpen(false)}
            />
          ) : (
            // The read has not landed, or landed unusable. Said rather than left
            // blank: a drawer that opens onto nothing reads as a broken button,
            // and the reader has already committed a tap to get here.
            <p>
              {approval.isPending
                ? t("worklist.decision.loading")
                : t("worklist.decision.unavailable")}
            </p>
          )}
        </div>
      </Modal>
    </div>
  );
}
