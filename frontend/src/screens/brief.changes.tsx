// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
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
              <li className="magic-line" key={receipt.id}>
                <div className="magic-line-text">
                  {receipt.subject?.type === "deal" && (
                    <EntityRef kind="deal" id={receipt.subject.id} />
                  )}
                  <p>{receiptSummary(receipt, t, locale, recordZone)}</p>
                  <p className="t-caption">
                    {formatDateTime(receipt.occurred_at, locale, viewerZone())}
                  </p>
                </div>
                <div className="magic-line-back">
                  <ReceiptReview receipt={receipt} />
                </div>
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
