// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { EntityRef } from "./entityref";
import { listReadState } from "./worklist.listread";
import { useHandledForYou } from "./worklist.queries";
import { ReceiptReview } from "./worklist.receiptreview";

export function BriefChanges() {
  const t = useT();
  const { locale } = useLocale();
  const query = useHandledForYou();
  const receipts = query.data?.receipts;
  return (
    <Panel title={t("brief.changes.title")}>
      <SurfaceState
        state={listReadState(query, receipts)}
        emptyLabel={t("brief.changes.empty")}
        loadingLabel={t("worklist.handled.loading")}
        detail={{ onRetry: () => void query.refetch() }}
      >
        {receipts?.map((receipt) => (
          <PanelRow key={receipt.id}>
            <div className="brief-change">
              {receipt.subject?.type === "deal" && (
                <EntityRef kind="deal" id={receipt.subject.id} />
              )}
              <p className="t-body">{receipt.summary}</p>
              <p className="t-caption">
                {formatDateTime(receipt.occurred_at, locale, viewerZone())}
              </p>
              <ReceiptReview receipt={receipt} />
            </div>
          </PanelRow>
        ))}
      </SurfaceState>
      {query.data?.truncated && (
        <PanelBody>
          <p className="t-caption">{t("worklist.handled.truncated")}</p>
        </PanelBody>
      )}
    </Panel>
  );
}
