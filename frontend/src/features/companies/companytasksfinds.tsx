// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Children } from "react";
import type { OpenReceipt } from "../../components/citations";
import { CompanySuggestions } from "../../components/dealsuggestion";
import { useAccountScan } from "./accountscan";
import {
  type Company360,
  proposesAStep,
  type SuggestionAction,
} from "./company360";
import { SignalsSection } from "./companysignals";
import { NeedsList, useTodayReading } from "./companytoday";

/**
 * What Margince found on the account beyond the proposed steps. That is the
 * agent's other finds and the manual moves, a deal the evidence suggests, and
 * the open signals. The proposed steps stand above in their own panel. This list
 * leaves them out.
 */
export function CompanyTasksFinds({
  companyId,
  view,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPerform,
}: Readonly<{
  companyId: string;
  view: Company360;
  onPrepareMeeting: (activityId: string) => void;
  onDraftTo: (contactId: string) => void;
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail: (activityId: string) => void;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const scan = useAccountScan(companyId, true);
  const reading = useTodayReading({
    companyId,
    view,
    loading: false,
    failed: false,
    onPrepareMeeting,
    onDraftTo,
    onOpenRecord,
    onOpenReceipt,
    onOpenEmail,
    onPerform,
    scan,
    keep: (suggestion) => !proposesAStep(suggestion),
  });
  // An empty list would read "nothing needs you" beside a late promise.
  const listed =
    reading.state === "ready" &&
    (Children.toArray(reading.rows).length > 0 || !reading.complete);
  return (
    <>
      {listed && <NeedsList reading={reading} />}
      <CompanySuggestions companyId={companyId} />
      <SignalsSection companyId={companyId} />
    </>
  );
}
