import { useRef, useState } from "react";
import { RecordTeam } from "../../components/recordteam";
import type { useTaskUpdate } from "../../components/taskactions";
import { EmptyState } from "../../design-system/atoms";
import { liveProjects } from "../../design-system/projectpicker";
import type { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import type { CompanyTab } from "../../lib/companytab";
import { useRecordZone } from "../../lib/recordzone";
import { ContactMeetingBrief } from "../brief/meetingbrief";
import { ComposeModal } from "../mail/compose";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { BillingContactsPanel } from "../projects/billingcontacts";
import { PartnerTab } from "../projects/partners";
import { openCitation } from "../records/record360";
import type { Company, Company360View } from "./companies";
import type { useCitedReceipt } from "./companies.chronology";
import { CompanyOverviewStack } from "./companies.overview";
import { CompanyProfileTab } from "./companies.profile";
import { CompanyDealsAndTasksTabs } from "./companies.tabs";
import { CompanyReachCard } from "./company-reach";
import { recordNamesIn, type SuggestionAction } from "./company360";
import { CompanyContactsList } from "./companycontacts/contacts";
import { CoverageBand } from "./companycontacts/summary";
import { CompanyContractsCard } from "./companycontracts";
import { CompanyDocumentsCard } from "./companydocuments";
import { EvidenceModal } from "./companyevidence";
import { CompanyFinanceCard } from "./companyfinance";
import "./company360.css";
import "./companytabs.css";

// What the composer is anchored on. A reply answers the message it names. An
// account-started message names the contact it goes to, because it has no
// thread to inherit a recipient from.
export type ComposeAnchor =
  | { kind: "reply"; id: string }
  | { kind: "account"; id: string };

// A suggestion action kind the page has no handler for. TypeScript refuses a
// new contract kind at the switch that calls this. The runtime throw is for a
// payload the build never saw.
export function unreachableAction(kind: never): never {
  throw new Error(`no surface performs the suggestion action ${String(kind)}`);
}

// The composer, opened on whichever anchor the page holds. Extracted so the
// page does not carry a branch per anchor kind in its own JSX.
function AccountComposer({
  anchor,
  open,
  companyId,
  onClose,
}: Readonly<{
  anchor: ComposeAnchor;
  /** Closed, it stays mounted so the drawer can animate out. */
  open: boolean;
  companyId: string;
  onClose: () => void;
}>) {
  const reply = anchor.kind === "reply";
  return (
    <ComposeModal
      activityId={reply ? anchor.id : undefined}
      contactId={reply ? undefined : anchor.id}
      entityType="company"
      entityId={companyId}
      kind="email"
      open={open}
      onClose={onClose}
    />
  );
}

// CompanyRecordBody is every tab's content below the strip and the tab bar.
// It lives apart from CompanyPage, so that render stays a layout (the header
// and the rail) rather than the seven-way branch beneath it.
export function CompanyRecordBody({
  company,
  view,
  loading,
  failed,
  tab,
  onTab,
  t,
  receipt,
  composing,
  onCompose,
  onPerform,
  openTaskId,
  onOpenTask,
  onOpenTaskFromToday,
  onLogActivity,
  taskUpdate,
  onOpenHistory,
  refusedReasonId,
}: Readonly<{
  company: Company;
  view?: Company360View;
  // The composite read's own pending flag, for every card below that reads
  // `view` with no skeleton guard of its own. sectionState's doc says why
  // "undefined view" is not one fact.
  loading: boolean;
  failed: boolean;
  tab: CompanyTab;
  onTab: (next: CompanyTab) => void;
  t: ReturnType<typeof useT>;
  receipt: ReturnType<typeof useCitedReceipt>;
  composing: ComposeAnchor | null;
  onCompose: (anchor: ComposeAnchor | null) => void;
  onPerform: (action: SuggestionAction) => void;
  openTaskId: string | null;
  onOpenTask: (activityId: string | null) => void;
  // Opens a task the daily brief named, switching to the Tasks tab first.
  // `onOpenTask` above is for the Tasks tab's own rows, which need no switch.
  onOpenTaskFromToday: (activityId: string) => void;
  // Opens the header's Log-activity drawer, off the daily brief's own verb.
  onLogActivity: () => void;
  taskUpdate: ReturnType<typeof useTaskUpdate>;
  onOpenHistory: () => void;
  // The page's one sentence about why this account takes no changes, by id,
  // for the profile tab's relationship edges.
  refusedReasonId?: string;
}>) {
  // The anchor the composer is drawn from, kept after `composing` clears. The
  // drawer stays mounted to animate out, and dropping the anchor on close would
  // empty it in front of the reader. Null only before the first composer opens.
  const shownAnchor = useRef<ComposeAnchor | null>(null);
  const recordZone = useRecordZone();
  const canWriteCompany =
    useCanWriteRecord("company", company) && !company.archived_at;
  if (composing !== null) {
    shownAnchor.current = composing;
  }
  // The meeting whose brief is open. "Prepare meeting" opens the brief drawer,
  // not the composer: a reply to a room nobody has sat in yet prepares no one.
  const [preparing, setPreparing] = useState<string | null>(null);
  // The account's own names for the records the dossier cites, off the same
  // 360 the overview's own copy of the card reads.
  const dossierNames = recordNamesIn(view);
  return (
    <>
      {tab === "partner" && <PartnerTab companyId={company.id} />}
      {/* The partial-read notice stands in the overview's stack, not above it.
          The sentence and the column it qualifies are one body. Above it, the
          notice's card would meet the stack's first pane at the border. */}
      {tab === "overview" && (
        <div className="record-stack">
          {failed && <EmptyState>{t("co.partial")}</EmptyState>}
          {/* What the rep must act on comes before what only reports state. It
              is built from sections the page already read: open tasks, the
              calendar, what changed since the last visit, and the suggestions.
              They come in the order a rep works them, with facts, assessments
              and recommendations labelled apart. */}
          <CompanyOverviewStack
            company={company}
            view={view}
            loading={loading}
            failed={failed}
            onOpenTab={onTab}
            taskUpdate={taskUpdate}
            onOpenRecord={openCitation}
            onOpenEmail={receipt.openEmail}
            onOpenTasks={() => onTab("tasks")}
            onOpenTask={onOpenTaskFromToday}
            onLogActivity={onLogActivity}
            onDraftTo={(id) => onCompose({ kind: "account", id })}
          />
        </div>
      )}
      <ContactMeetingBrief
        activityId={preparing}
        open={preparing !== null}
        onClose={() => setPreparing(null)}
        projects={liveProjects(view?.projects)}
        onOpenEmail={receipt.openEmail}
      />
      {/* Deals and Tasks have their own tabs, off the overview. A reader who
          came for the commercial picture or the open work should not scroll
          past the day's brief to find either. */}
      <CompanyDealsAndTasksTabs
        tab={tab}
        company={company}
        view={view}
        loading={loading}
        failed={failed}
        openTaskId={openTaskId}
        onOpenTask={onOpenTask}
        onOpenRecord={openCitation}
        onOpenReceipt={receipt.open}
        onOpenEmail={receipt.openEmail}
        onPrepareMeeting={setPreparing}
        onDraftTo={(id) => onCompose({ kind: "account", id })}
        onPerform={onPerform}
        taskUpdate={taskUpdate}
      />
      {/* The composer, anchored on the message a draft_reply suggestion named.
          It is the same modal the timeline's own Reply opens. The advice
          leads to it rather than to a second way to answer. */}
      {shownAnchor.current !== null && (
        <AccountComposer
          anchor={shownAnchor.current}
          open={composing !== null}
          companyId={company.id}
          onClose={() => onCompose(null)}
        />
      )}
      {/* The Contacts tab gives the account team the whole middle column, with
          room for the title and the last exchange beside each name. The
          rail's capped summary stands beside it. A top-3 glance is the
          reader's anchor across tabs, not a second copy of the roster. */}
      {tab === "contacts" && !company.archived_at && (
        <div className="co-tab-stack">
          {/* The account's contacts, ranked and paged. One representation, not
              three. A roster card, a connections card and a diagram would each
              answer "who works here" in a different shape. The reader's
              question is which of them to write to. */}
          {!view?.sections_omitted?.includes("contacts") && (
            <CompanyContactsList
              companyId={company.id}
              bandSlot={(narrow) => (
                <CoverageBand
                  companyId={company.id}
                  accountName={company.display_name}
                  onNarrow={narrow}
                />
              )}
            />
          )}
          {/* Who handles the invoices belongs beside the roster, not only under
              Finance. Naming an invoice recipient is a fact about the account's
              contacts, and the reader working them is the one who knows it.
              It is the same panel the Finance tab shows. It reads its rows off
              the Company360 the tab already holds, so the write path and its
              refusals stay in one place. */}
          {!view?.sections_omitted?.includes("billing_contacts") && (
            <BillingContactsPanel
              contacts={view?.billing_contacts}
              companyId={company.id}
            />
          )}
          {/* Our side of the account: who holds which responsibility, and
              which colleagues can reach those not yet on file. */}
          <RecordTeam
            recordType="company"
            recordId={company.id}
            readOnly={!canWriteCompany}
          />
          <CompanyReachCard companyId={company.id} />
        </div>
      )}
      {/* Files get the whole column on their own tab, which is what the mockup
          gives them. The grid keeps its compact card for the reader who only
          wants to know whether there is paperwork at all. */}
      {/* The money gets the whole column. What is overdue, what was invoiced
          over the year, and how this account pays are three readings. A rep
          opens the page with a question about them, not on the way past the
          day's brief. The card keeps its own lifecycle-varying title inside
          the tab. A former customer's figures read under "Finance
          (historical)". The tab strip cannot say that, because a tab label
          names a place, not its qualifier. */}
      {tab === "finance" && !company.archived_at && (
        <CompanyFinanceCard
          companyId={company.id}
          lifecycle={company.lifecycle}
        />
      )}
      {tab === "documents" && (
        // The tab's stacked column, so the two panels are spaced like every
        // other pair in this record, not touching.
        <div className="co-tab-stack">
          {/* The agreements come first: contract paper is what a reader opens
              this tab for. The library beneath holds everything else, since it
              withholds the paper already read on an agreement's own row. Two
              panels, and no file on both of them. */}
          {!company.archived_at && (
            <CompanyContractsCard companyId={company.id} />
          )}
          <CompanyDocumentsCard
            companyId={company.id}
            refusedReasonId={refusedReasonId}
            onOpenEmail={receipt.openEmail}
          />
        </div>
      )}
      {receipt.cited && (
        <EvidenceModal
          companyId={company.id}
          cited={receipt.cited}
          onClose={receipt.close}
          onStep={receipt.step}
        />
      )}
      {/* The message a citation opened. Beside the receipt modal above and for
          the same reason: both are what a chip on this page opens into. */}
      <OpenEmailDrawer
        activityId={receipt.email}
        zone={recordZone}
        onClose={receipt.closeEmail}
      />
      <CompanyProfileTab
        active={tab === "profile"}
        company={company}
        view={view}
        onOpenHistory={onOpenHistory}
        refusedReasonId={refusedReasonId}
        nameOf={dossierNames}
        onOpenRecord={openCitation}
        onOpenReceipt={receipt.open}
        onOpenEmail={receipt.openEmail}
      />
    </>
  );
}
