// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { OpenReceipt } from "../../components/citations";
import { CompanySuggestions } from "../../components/dealsuggestion";
import { Panel, PanelGroupHead } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useAccountScan } from "./accountscan";
import type { Company360, SuggestionAction } from "./company360";
import { SignalsSection } from "./companysignals";
import { CompanyTaskRows } from "./companytaskrows";
import { NeedsList, useTodayReading } from "./companytoday";
import "./companytabs.css";

type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

/**
 * The Tasks tab as one list in three groups: the account's open tasks, what
 * Margince suggests doing next, and the signals read off its conversations.
 * The foot says what the suggestions were read from.
 */
export function CompanyTaskList({
  companyId,
  view,
  onOpenTask,
  renderTick,
  renderAction,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPerform,
}: Readonly<{
  companyId: string;
  view: Company360;
  onOpenTask?: (step: NextStep) => void;
  renderTick?: (step: NextStep) => ReactNode;
  renderAction?: (step: NextStep) => ReactNode;
  onPrepareMeeting: (activityId: string) => void;
  onDraftTo: (contactId: string) => void;
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail: (activityId: string) => void;
  onPerform: (action: SuggestionAction) => void;
}>) {
  const t = useT();
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
  });
  return (
    <Panel
      title={t("co.next.title")}
      className="co-tasklist"
      footer={reading.state === "ready" ? reading.footer : undefined}
    >
      <PanelGroupHead title={t("co.tasks.open")} level="h3" />
      <CompanyTaskRows
        view={view}
        onOpenTask={onOpenTask}
        renderTick={renderTick}
        renderAction={renderAction}
      />
      <PanelGroupHead title={t("co.tasks.suggested")} level="h3" />
      <NeedsList reading={reading} />
      <CompanySuggestions companyId={companyId} />
      <SignalsSection companyId={companyId} />
    </Panel>
  );
}
