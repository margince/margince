// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { Deal } from "../deals";
import { ArchivedDealBand } from "./dealrestore";

// Why an archived deal takes no changes, said once across the header so every
// refused control can point at it, with the way back.
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
  // A live deal this reader may only view says so in the head's View only
  // pill, which holds the sentence under `reasonId`.
  if (reason === undefined || !deal.archived_at) {
    return marginceLine;
  }
  const sentence = (
    <ArchivedDealBand deal={deal} reason={reason} reasonId={reasonId} />
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
