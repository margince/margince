// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useAccountScan } from "./accountscan";
import { ThreadFold } from "./company/glance";
import { recordNamesIn, StateStrip, type SuggestionAction } from "./company360";
import { DeepReadPanel } from "./companydeepread";
import type { CompanyTab } from "./companytab";
import { Company360Call, NeedsList, useTodayReading } from "./companytoday";
import { sinceLastVisitFooter } from "./companywork";
import { useRoster } from "./entityref";
import { type OpenReceipt, RecordSpine, WrittenBy } from "./record360";

type Company = components["schemas"]["Company"];
type Company360View = components["schemas"]["Company360"];

export type CompanyOverviewProps = Readonly<{
  company: Company;
  view?: Company360View;
  // Decided by the page, so the research offer stands on this column or on the
  // Profile tab, never on both and never on neither.
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
      {/* The readings lead, under the bar that chose this tab: five doors into
          the tabs that hold their rows. They belong to THIS tab rather than to
          the record — the Contacts tab is a roster and the Documents tab is a
          filing cabinet, and a row of account readings over either is a header
          for a page it is not describing. */}
      {view && (
        <StateStrip companyId={company.id} view={view} onOpenTab={onOpenTab} />
      )}
      {/* An account with nothing on file is asked a different question from a
          running one: not "what should you do here" but "shall Margince go and
          find out who these contacts are". So the research offer LEADS the
          column on such an account, and stands down the moment there is
          anything to read: on a live account the lead is the 360 below, and
          two leads is none.

          An account already RESEARCHED has answered that question too, even
          when the read filled nothing in. The offer belongs on Profile from
          then on, with the record's other tools, where a rep goes to run it
          again — see offerResearchOnOverview. */}
      {offerResearch && <DeepReadPanel companyId={company.id} />}
      {/* The call leads at full measure: the word, its sentence, the three
          dimensions, the spine and the folded thread. What moved since the
          reader's last visit rides in its foot. */}
      <Company360Call
        reading={reading}
        name={company.display_name}
        title={t("company.brief.title")}
        titleAction={
          view && (
            <>
              {/* The reading's own date, and only where the payload carries a
                  date this can read. A server one release out of step answers
                  the 360 without `as_of`, and Intl refuses an unparseable
                  value by throwing, which took the whole record down over a
                  line that only qualifies the head. */}
              {!Number.isNaN(Date.parse(view.as_of)) && (
                <span className="t-caption">
                  {t("contact.brief.updatedAt", {
                    when: formatDate(view.as_of, locale, recordZone),
                  })}
                </span>
              )}
              {/* The 360 call is a composition read off this account's own
                  records rather than model prose, so its claim of authorship
                  is the deterministic one — the same distinction the sources
                  under the dossier and the brief already carry. */}
              <WrittenBy by="deterministic" />
            </>
          )
        }
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
        {/* Keyed on the account, so its fold is the account's own. The page
              stays mounted while the route swaps companies, and without
              this a thread a reader OPENED on one company arrives open on the
              next — carrying one account's reading into another's, and
              spending the glance the fold is closed to protect. */}
        <ThreadFold
          key={company.id}
          view={view}
          loading={loading}
          onOpenHistory={onOpenHistory}
          onOpenRecord={onOpenRecord}
          onOpenEmail={onOpenEmail}
        />
      </Company360Call>
      {/* One column under the 360, full width at every measure: content-driven
          cards of unequal height never share a row, so a tall needs list and a
          short Ask box beside it would leave the short one's row half empty.
          The order is the order a rep works them: what needs a contact, then
          what the account is, then the questions the 360 already answers. The
          money and the pipeline read on the Deals tab now, and the rail's own
          DealsSection stands on every tab: a third copy of them here would be
          a figure the reader has to reconcile against the other two. */}
      <NeedsList reading={reading} onOpenTasks={onOpenTasks} />
    </>
  );
}
