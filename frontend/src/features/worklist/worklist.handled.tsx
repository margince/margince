// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the product did on this reader's behalf.
//
// Every other panel on this page asks for something. This one asks for nothing:
// it is the receipt a reader checks, and the reason the acts above it are safe
// to take at all. A product that acts autonomously and never says what it acted
// on is asking for trust it has not earned.
//
// NO VERBS, and that is the point rather than an omission. A receipt carries no
// action because the work is already done — a row here offering "complete" would
// ask the reader to redo it, on the one surface that exists to tell them they
// need not.
//
// LAST ON THE PAGE, and open. It sits below every other panel because a reader
// opens this page to find what to do next, and a list of what is finished
// answers a different question — one worth having and not worth leading with.
// Position is what says that; folding it away said something else, that the
// receipt is optional reading, on the one surface whose whole job is to be
// checked. It wears the team board's chrome for the same reason every other
// panel here does: one shape, so a reader learns it once.

import { useEffect, useState } from "react";
import { BundleUndo } from "../../components/marginchanges";
import { ReceiptDetail } from "../../components/receiptdetail";
import { ReceiptReview } from "../../components/receiptreview";
import { ReceiptsMore } from "../../components/receiptsmore";
import { AppLink } from "../../design-system/applink";
import { Disclosure } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { useJumpTo } from "../../design-system/jumpbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  shownUndoable,
  usePendingMarginceChanges,
} from "../../lib/marginchanges";
import { receiptBasis, receiptSummary } from "../../lib/receiptcopy";
import { isReviewableReceipt, receiptAuditId } from "../../lib/receiptreview";
import { useRememberedUndoRun } from "../../lib/undorun";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { listReadState } from "../../lib/worklist.listread";
import { subjectHref } from "../../lib/worklistcopy";
import {
  CHANGE_HISTORY_SECTION,
  WORKLIST_SECTION_PARAM,
} from "../../lib/worklistrouting";
import { AFTER_THE_DAY } from "./worklist.layout";
import { type Receipt, useHandledForYou } from "./worklist.queries";

/** The id a link to the Change history panel lands on. */
export const CHANGE_HISTORY_ID = "worklist-change-history";

export function HandledForYouPanel() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const handled = useHandledForYou();
  const pendingChanges = usePendingMarginceChanges().data;
  const lastRun = useRememberedUndoRun() !== null;
  const receipts = handled.data?.receipts;
  // Off the LIST, not off the query's flags alone. `receipts` is empty on most
  // days — the contract says so — and a state read from `isPending`/`isError`
  // called that `ready`, which drew a table's header row over no rows: three
  // column names and a silence, where the sentence saying nothing was done on
  // the reader's behalf is the whole answer.
  const state = listReadState(handled, receipts);
  const arrived = useArrivalJump(state !== "loading");
  // A day with nothing done draws no panel: the empty sentence sat beside the
  // queue's own on every quiet day. A failed read still shows, with its retry.
  // A link that named this panel lands on its empty sentence.
  if (state === "empty" && !arrived && !lastRun) return null;
  // Undo sends only the changes this list shows that the server still allows.
  const shown = new Set(
    (receipts ?? [])
      .filter(isReviewableReceipt)
      .map(receiptAuditId)
      .filter((id): id is string => id !== undefined),
  );
  const undoable = (pendingChanges?.bundles ?? []).flatMap((bundle) => {
    const audits = shownUndoable(bundle, shown);
    return audits.length > 0 ? [{ ...bundle, audit_log_ids: audits }] : [];
  });
  const panel = (
    <Panel
      title={t("worklist.handled.title")}
      // HOW MUCH was done, in the band that belongs to the whole panel.
      //
      // Only over a read that ANSWERED, and only over one that reached its own
      // end. Both halves are the same wrong number in the same direction. A
      // cached list outlives the refetch that failed, so a count taken off the
      // rows alone stood in this band while the body said the receipts could
      // not be read — and of those two the number is the one a reader believes.
      // `truncated` makes the length a floor for the same reason, and a receipt
      // surface printing a floor as a count tells a reader they have seen
      // everything, which is the one thing it must never cause. The caveat in
      // the body is what they get instead.
      //
      // `ready` already means there is a non-empty list, so `receipts` here
      // narrows the type rather than deciding anything.
      footer={
        state === "ready" && receipts && !handled.data?.truncated
          ? t("worklist.handled.count", {
              count: formatNumber(receipts.length, locale),
            })
          : undefined
      }
    >
      {/* The table carries its own cell padding but not the panel's inset, so
          it sits in a `PanelBody` with the truncation caveat rather than
          full-bleed against the panel's own edges. */}
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("worklist.handled.empty")}
          loadingLabel={t("worklist.handled.loading")}
          detail={{ onRetry: () => void handled.refetch() }}
        >
          {/* `ready` already means there are rows — the state above is derived
              from the list — so this narrows the type rather than deciding
              anything. A response carrying no list at all resolved to
              `unavailable` and never reaches here: mapping over the absence
              would take the page down with a type error, and calling it a quiet
              day would report a clear receipt over an answer nobody could
              read. */}
          {(undoable.length > 0 || lastRun) && (
            <BundleUndo
              bundles={undoable}
              remember
              label={plural("changes.undoDeals", undoable.length, {
                count: formatNumber(undoable.length, locale),
              })}
            />
          )}
          {receipts && (
            <Disclosure summary={t("worklist.handled.showHistory")}>
              <DataTable
                label={t("worklist.handled.title")}
                rows={receipts}
                rowKey={(row: Receipt) => row.id}
                columns={[
                  {
                    key: "summary",
                    header: t("worklist.handled.what"),
                    render: (row: Receipt) => (
                      <>
                        {receiptSummary(row, t, locale, readerZone)}
                        {receiptBasis(row, t, locale) && (
                          <p className="t-caption">
                            {receiptBasis(row, t, locale)}
                          </p>
                        )}
                        <ReceiptDetail receipt={row} />
                      </>
                    ),
                  },
                  {
                    key: "subject",
                    header: t("worklist.handled.about"),
                    render: (row: Receipt) => <ReceiptRecord receipt={row} />,
                  },
                  {
                    key: "when",
                    header: t("worklist.handled.when"),
                    render: (row: Receipt) =>
                      formatDateTime(row.occurred_at, locale, readerZone),
                  },
                  {
                    // The one verb this panel carries, and only on the rows
                    // that earned it. A receipt for a decision somebody made
                    // renders an empty cell: the work was agreed to, so there
                    // is nothing here to take back.
                    key: "undo",
                    header: t("worklist.handled.wayBack"),
                    render: (row: Receipt) => <ReceiptReview receipt={row} />,
                  },
                ]}
              />
              {/* A bounded read is not the whole of what was done, so the
                  list ends in the way to the rest or in the caveat saying so. */}
              <ReceiptsMore handled={handled} />
            </Disclosure>
          )}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
  return (
    <div id={CHANGE_HISTORY_ID} tabIndex={-1} className={AFTER_THE_DAY}>
      {panel}
    </div>
  );
}

// A link that names the Change history lands here once the panel has read its
// list. The dial is then cleared, so the same link moves the reader again.
// Answers whether this panel was opened by such a link.
function useArrivalJump(ready: boolean): boolean {
  const [params] = useUrlParams();
  const asked = params.get(WORKLIST_SECTION_PARAM) === CHANGE_HISTORY_SECTION;
  const [arrived] = useState(asked);
  const jump = useJumpTo();
  useEffect(() => {
    if (!asked || !ready) return;
    jump(CHANGE_HISTORY_ID);
    replaceDial(WORKLIST_SECTION_PARAM, undefined);
  }, [asked, ready, jump]);
  return arrived;
}

// The record an act changed, named and linked so the reader can judge Undo
// against Accept. A subject without a label is one the reader may not read: it
// gets neither the name nor a link, only the fact that a record was involved.
function ReceiptRecord({ receipt }: { readonly receipt: Receipt }) {
  const t = useT();
  const subject = receipt.subject;
  if (!subject) return <>{t("worklist.handled.noRecord")}</>;
  if (!subject.label) return <>{t("worklist.handled.hiddenRecord")}</>;
  const href = subjectHref(receipt);
  if (!href) return <>{subject.label}</>;
  return (
    <AppLink className="entity-link" href={href}>
      {subject.label}
    </AppLink>
  );
}
