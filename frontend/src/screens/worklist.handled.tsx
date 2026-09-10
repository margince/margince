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

import { DataTable } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { AFTER_THE_DAY } from "./worklist.layout";
import { listReadState } from "./worklist.listread";
import { type Receipt, useHandledForYou } from "./worklist.queries";
import { ReceiptUndo } from "./worklist.receiptundo";

export function HandledForYouPanel() {
  const t = useT();
  const { locale } = useLocale();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = viewerZone();
  const handled = useHandledForYou();
  const receipts = handled.data?.receipts;
  // Off the LIST, not off the query's flags alone. `receipts` is empty on most
  // days — the contract says so — and a state read from `isPending`/`isError`
  // called that `ready`, which drew a table's header row over no rows: three
  // column names and a silence, where the sentence saying nothing was done on
  // the reader's behalf is the whole answer.
  const state = listReadState(handled, receipts);
  return (
    <Panel
      className={AFTER_THE_DAY}
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
          {receipts && (
            <>
              <DataTable
                label={t("worklist.handled.title")}
                rows={receipts}
                rowKey={(row: Receipt) => row.id}
                columns={[
                  {
                    key: "summary",
                    header: t("worklist.handled.what"),
                    render: (row: Receipt) => row.summary,
                  },
                  {
                    key: "subject",
                    header: t("worklist.handled.about"),
                    // Only where the act named a record. Not every approval is
                    // about one, and an absent subject is a real state — the row
                    // reads as its summary alone rather than inventing something
                    // to point at.
                    render: (row: Receipt) =>
                      row.subject?.label ?? t("worklist.handled.noRecord"),
                  },
                  {
                    key: "when",
                    header: t("worklist.handled.when"),
                    render: (row: Receipt) =>
                      formatDateTime(row.occurred_at, locale, zone),
                  },
                  {
                    // The one verb this panel carries, and only on the rows
                    // that earned it. A receipt for a decision somebody made
                    // renders an empty cell: the work was agreed to, so there
                    // is nothing here to take back.
                    key: "undo",
                    header: t("worklist.handled.wayBack"),
                    render: (row: Receipt) => <ReceiptUndo receipt={row} />,
                  },
                ]}
              />
              {/* A bounded read is not the whole of what was done. A reader who
                took this list for everything would close the page believing
                they had seen it all, which is the one thing a receipt surface
                must not cause. */}
              {handled.data?.truncated && (
                <p className="t-caption">{t("worklist.handled.truncated")}</p>
              )}
            </>
          )}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
