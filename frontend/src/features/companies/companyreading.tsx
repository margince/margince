// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useRoster } from "../../components/entityref";
import { formatDayMonth } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import {
  type OpenReceipt,
  RecordSpine,
  WrittenBy,
} from "../../screens/record360";
import { useAccountScan } from "../capture/accountscan";
import { ThreadFold } from "./company/glance";
import { recordNamesIn, StateStrip, type SuggestionAction } from "./company360";
import { DeepReadPanel } from "./companydeepread";
import type { CompanyTab } from "./companytab";
import { Company360Call, NeedsList, useTodayReading } from "./companytoday";
import { sinceLastVisitFooter } from "./companywork";

type Company = components["schemas"]["Company"];
type Company360View = components["schemas"]["Company360"];

export type CompanyOverviewProps = Readonly<{
  company: Company;
  view?: Company360View;
  // Decided by the page, so the research offer never stands on this column and
  // on the Profile tab at once.
  offerResearch: boolean;
  // The composite read's own pending flag (see CompanyRecordBody).
  loading: boolean;
  failed: boolean;
  onOpenHistory: () => void;
  onOpenRecord: (entityType: string, entityId: string) => void;
  // The page's own drawers: the profile tab cites the same records, and two
  // owners would open two drawers over each other.
  onOpenReceipt: OpenReceipt;
  onOpenEmail: (activityId: string) => void;
  onOpenTasks: () => void;
  // The leading card's own fallback verbs, threaded to `useTodayReading`: a
  // named task's detail modal, and the header's Log-activity drawer.
  onOpenTask: (activityId: string) => void;
  onLogActivity: () => void;
  // Opens the meeting brief for the day's meeting, not the composer.
  onPrepareMeeting: (activityId: string) => void;
  onDraftTo: (contactId: string) => void;
  onOpenTab: (tab: CompanyTab) => void;
  onPerform: (action: SuggestionAction) => void;
}>;

// The Overview's head. Every part of it reads the 360 or the money summary, and
// the server refuses both for an archived account, so the page leaves it out.
export function CompanyReading({
  company,
  view,
  loading,
  failed,
  onOpenHistory,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onOpenTasks,
  onOpenTask,
  onLogActivity,
  onPrepareMeeting,
  onDraftTo,
  onOpenTab,
  onPerform,
  offerResearch,
}: CompanyOverviewProps) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Ids resolve against the account's own records and the workspace roster.
  // It walks the roster rather than reading useMemberNames by id, because
  // `nameOf` is called per value as the reading's rows render.
  const roster = useRoster("user", true);
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  const records = recordNamesIn(view);
  const nameOf = (entityType: string, entityId: string) =>
    entityType === "user"
      ? colleagues.get(entityId)
      : records(entityType, entityId);
  // The reader's scan of this account, asked for once the 360 has answered:
  // the scan reads the same records.
  const scan = useAccountScan(company.id, view !== undefined);
  // One reading drawn in two panes, the call and the needs list. Computed once
  // here, so the verdict and the queue cannot disagree.
  const reading = useTodayReading({
    companyId: company.id,
    view,
    loading,
    failed,
    onPrepareMeeting,
    onDraftTo,
    onOpenRecord,
    onOpenReceipt,
    onOpenEmail,
    onPerform,
    onOpenTask,
    onLogActivity,
    scan,
  });
  return (
    <>
      {/* On this tab rather than the record: over a roster or a filing cabinet,
          a row of account readings heads a page it does not describe. */}
      {view && (
        <StateStrip companyId={company.id} view={view} onOpenTab={onOpenTab} />
      )}
      {/* An account with nothing on file asks whether to research it, so the
          offer leads; once there is anything to read, the call below leads. */}
      {offerResearch && <DeepReadPanel companyId={company.id} />}
      <Company360Call
        reading={reading}
        name={company.display_name}
        // A dateline over the call: what this is, and as of when. Intl throws
        // on an unparseable date, and a server one release out of step
        // answers without `as_of`, so the date waits for one it can read.
        title={
          view && !Number.isNaN(Date.parse(view.as_of))
            ? t("record.howItStands", {
                when: formatDayMonth(view.as_of, locale, recordZone),
              })
            : t("company.brief.title")
        }
        // Composed from the account's own records, not model prose.
        titleAction={view && <WrittenBy by="deterministic" />}
        scale="compact"
        footer={sinceLastVisitFooter(view)}
      >
        <RecordSpine
          source={view}
          commercial={view?.state_strip?.commercial}
          // A link carries only an id, so contacts are named off the account's
          // roster. Colleagues come from the workspace roster: whoever held a
          // meeting is one of ours.
          nameOf={nameOf}
          onOpenEmail={onOpenEmail}
        />
        {/* Keyed on the account: the page stays mounted across companies, and a
            thread opened on one must not arrive open on the next. */}
        <ThreadFold
          key={company.id}
          view={view}
          loading={loading}
          onOpenHistory={onOpenHistory}
          onOpenRecord={onOpenRecord}
          onOpenEmail={onOpenEmail}
        />
      </Company360Call>
      {/* Full width at every measure: cards of unequal height never share a
          row, or the shorter one's row is left half empty. */}
      <NeedsList reading={reading} onOpenTasks={onOpenTasks} />
    </>
  );
}
