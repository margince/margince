// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDateTime,
  formatDayMonth,
  formatTimeOfDay,
} from "../format/format";
import { dayInZone, viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { EntityRef } from "./entityref";
import { listReadState } from "./worklist.listread";
import { useHandledForYou } from "./worklist.queries";
import { receiptSummary } from "./worklist.receiptcopy";
import { ReceiptReview } from "./worklist.receiptreview";

// The changes made for the reader that they can still accept or take back, as
// one group of Home's receipt. Drawn only when there is one: the group sits
// under a summary that already answers for the quiet day, and a heading over
// "none" was a second card saying nothing.
export function BriefChanges() {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const zone = viewerZone();
  const query = useHandledForYou();
  const receipts = query.data?.receipts;
  const state = listReadState(query, receipts);
  if (state === "loading" || state === "empty") {
    return null;
  }
  return (
    <>
      <PanelGroupHead title={t("brief.changes.title")} level="h3" />
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel=""
          loadingLabel={t("worklist.handled.loading")}
          detail={{ onRetry: () => void query.refetch() }}
        >
          <ul className="magic-lines" aria-label={t("brief.changes.title")}>
            {receipts?.map((receipt) => (
              // The receipt's own row, with no mark: a change carries no
              // actor, and the mark's colour is a claim about who acted.
              <li className="magic-line" key={receipt.id}>
                <p className="magic-line-text">
                  {receiptSummary(receipt, t, locale, recordZone)}{" "}
                  {receipt.subject?.type === "deal" && (
                    <span className="magic-line-subject">
                      <EntityRef kind="deal" id={receipt.subject.id} />
                    </span>
                  )}
                </p>
                <div className="magic-line-controls">
                  <ReceiptReview receipt={receipt} />
                </div>
                <time
                  className="t-caption magic-line-when"
                  dateTime={receipt.occurred_at}
                  title={formatDateTime(receipt.occurred_at, locale, zone)}
                >
                  {sameDay(receipt.occurred_at, query.data?.as_of, zone)
                    ? formatTimeOfDay(receipt.occurred_at, locale, zone)
                    : formatDayMonth(receipt.occurred_at, locale, zone)}
                </time>
              </li>
            ))}
          </ul>
          {query.data?.truncated && (
            <p className="t-caption">{t("worklist.handled.truncated")}</p>
          )}
        </SurfaceState>
      </PanelBody>
    </>
  );
}

// A change from today shows its hour, an older one its day: the list spans as
// many days as changes have waited, and the hour of one from last week says
// less than which day it was.
function sameDay(iso: string, asOf: string | undefined, zone: string): boolean {
  return (
    asOf !== undefined &&
    dayInZone(Date.parse(iso), zone) === dayInZone(Date.parse(asOf), zone)
  );
}
