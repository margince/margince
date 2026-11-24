// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import { ReceiptDetail } from "../../../components/receiptdetail";
import { ReceiptVerbs } from "../../../components/receiptverbs";
import { Callout } from "../../../design-system/callout";
import { formatDayMonth } from "../../../format/format";
import { type Translator, useLocale, useT } from "../../../i18n";
import type { Receipt } from "../../../lib/handled";
import { currentMarginceChange } from "../../../lib/marginchanges";
import { receiptSummary } from "../../../lib/receiptcopy";
import { isOpenReceipt } from "../../../lib/receiptstate";
import { useViewerZone } from "../../../lib/viewerzone";
import { ReopenAction } from "./dealreopen";

type Stage = components["schemas"]["Stage"];
type Deal = components["schemas"]["Deal"];

function lineTitle(receipt: Receipt, t: Translator, day: string): string {
  switch (receipt.kind) {
    case "deal_open":
      return t("deal.marginceLine.opened", { date: day });
    case "deal_revive":
      return t("deal.marginceLine.reopened", { date: day });
    default:
      return t("deal.marginceLine.closed", { date: day });
  }
}

/**
 * Who closed, opened or reopened this deal, and why, under the header. While
 * the server offers Undo for it, the line carries the same Undo Home does. A
 * close that can no longer be undone offers Reopen. The line stays only while
 * it has a verb, so it never reads as a notice with nothing to do.
 */
export function DealMarginceLine({
  dealId,
  dealVersion,
  receipt,
  openStages,
  pipelineRetired,
  refusedReasonId,
}: Readonly<{
  dealId: string;
  dealVersion: number | undefined;
  receipt: Receipt;
  openStages: Stage[];
  pipelineRetired?: boolean;
  refusedReasonId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const writable = receipt.review?.writable === true;
  const open = isOpenReceipt(receipt);
  const closed = receipt.kind === "deal_close_lost";
  let actions = null;
  if (open) {
    actions = <ReceiptVerbs receipt={receipt} undoable={writable} />;
  } else if (closed && writable) {
    actions = (
      <ReopenAction
        dealId={dealId}
        dealVersion={dealVersion}
        openStages={openStages}
        pipelineRetired={pipelineRetired}
        disabledReasonId={refusedReasonId}
      />
    );
  }
  return (
    <Callout
      tone="ai"
      kind="standing"
      title={lineTitle(
        receipt,
        t,
        formatDayMonth(receipt.occurred_at, locale, zone),
      )}
      actions={actions}
    >
      {receiptSummary(receipt, t, locale, zone)}
      {!open && closed && ` ${t("deal.marginceLine.undoOff")}`}
      <ReceiptDetail receipt={receipt} />
    </Callout>
  );
}

/**
 * The deal's line, or `undefined` while Margince has nothing here to take
 * back. It is `undefined` and not an element that renders null, because the
 * record header reserves space for anything it is handed.
 */
export function marginceLineOf({
  deal,
  receipts,
  openStages,
  pipelineRetired,
  refusedReasonId,
}: Readonly<{
  deal: Deal;
  receipts: readonly Receipt[];
  openStages: Stage[];
  pipelineRetired?: boolean;
  refusedReasonId?: string;
}>): ReactNode | undefined {
  const receipt = currentMarginceChange(deal, receipts);
  return receipt ? (
    <DealMarginceLine
      dealId={deal.id}
      dealVersion={deal.version}
      receipt={receipt}
      openStages={openStages}
      pipelineRetired={pipelineRetired}
      refusedReasonId={refusedReasonId}
    />
  ) : undefined;
}
