// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EditAccessRequest } from "../../../components/editaccessrequest";
import type { Deal } from "../deals";
import { ArchivedDealBand } from "./dealrestore";

// One sentence for why this deal takes no changes, whichever reason applies,
// said once across the header so every refused control can point at it. An
// archived deal's band also offers the way back.
// Absent while the deal is live and the reader's: `undefined`, not an element
// that renders null, because RecordView reserves space for anything it is
// handed. A line always reserved would read as a record with nothing said.
export function dealBand({
  deal,
  reason,
  reasonId,
  marginceLine,
}: Readonly<{
  deal: Deal;
  reason: string | undefined;
  reasonId: string;
  // What Margince did to this deal and still offers to take back, or nothing.
  marginceLine: ReactNode | undefined;
}>): ReactNode | undefined {
  if (reason === undefined) {
    return marginceLine;
  }
  const sentence = deal.archived_at ? (
    <ArchivedDealBand deal={deal} reason={reason} reasonId={reasonId} />
  ) : (
    <>
      <p id={reasonId}>{reason}</p>
      <EditAccessRequest kind="deal" record={deal} />
    </>
  );
  return marginceLine ? (
    <>
      {sentence}
      {marginceLine}
    </>
  ) : (
    sentence
  );
}
