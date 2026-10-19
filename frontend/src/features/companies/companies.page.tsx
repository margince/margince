import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useId, useState } from "react";
import { usePageAside } from "../../components/pageaside";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { taskWriteKeys } from "../../lib/activitykeys";
import { invalidateCompany } from "../../lib/recordwritekeys";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { RecordHistoryTab } from "../../screens/history";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { useTaskUpdate } from "../worklist/taskactions";
import { WorklistReturnLink } from "../worklist/worklist.return";
import type { Company, Company360View } from "./companies";
import {
  CompanyRecordBody,
  type ComposeAnchor,
  unreachableAction,
} from "./companies.body";
import { useChronologySlots, useCitedReceipt } from "./companies.chronology";
import { CompanyActionBadges, useCompanyVerbRefusal } from "./companyheader";
import {
  type ActivityDrawer,
  CompanyHeaderActions,
} from "./companyheaderactions";
import { CompanyIdentityFacts } from "./companyheaderfacts";
import { CompanyNameLine } from "./companylifecycle";
import { CompanyMarks } from "./companymarks";
import { CompanyRail } from "./companyrail";
import type { CompanyTab } from "./companytab";

// openTaskId is scoped to the account being read, like the tab. The route
// swaps one company for another without unmounting this component, so a
// task modal opened on one account would stay over the next one.
//
// It is scoped to the tab too. The modal renders only on Tasks, so a tab change
// hides it without `onClose`, and a kept id would reopen it on return to Tasks.
function useOpenTaskId(
  recordId: string,
  tab: CompanyTab,
): [
  string | null,
  (next: string | null) => void,
  (onTab: CompanyTab, activityId: string) => void,
] {
  const [openTaskId, setOpenTaskId] = useState<string | null>(null);
  const [openTaskFor, setOpenTaskFor] = useState(recordId);
  const [openTaskOn, setOpenTaskOn] = useState(tab);
  if (openTaskFor !== recordId || openTaskOn !== tab) {
    setOpenTaskFor(recordId);
    setOpenTaskOn(tab);
    setOpenTaskId(null);
  }
  // Opens a task from off the Tasks tab. The reset above fires when the
  // route's tab changes, and would wipe this id on the next render. Priming
  // `openTaskOn` to the target tab keeps the guard's comparison true.
  const openTaskInTab = (nextTab: CompanyTab, activityId: string) => {
    setOpenTaskOn(nextTab);
    setOpenTaskId(activityId);
  };
  return [openTaskId, setOpenTaskId, openTaskInTab];
}

// CompanyPage is the page itself: identity and verbs at the top, then the
// tab body beside the rail. Every tab renders here. The rail belongs to the
// account, not a tab, so it stays mounted whichever tab is open.
export function CompanyPage({
  company,
  view,
  loading,
  failed,
  tab,
  tabs,
  onTab,
}: Readonly<{
  company: Company;
  view?: Company360View;
  loading: boolean;
  // The composite read failed, as distinct from "still loading" and "the
  // account is empty". All three would draw the same blank page, and only one
  // is a fact about the account.
  failed: boolean;
  tab: CompanyTab;
  // The rendered tab bar. It is handed down to the body rather than drawn
  // here, so the strip can lead and the bar sit beneath it.
  tabs: ReactNode;
  // An evidence mark can be clicked from the Partner tab, where its timeline
  // is not on screen. The page must return to the Overview before the filter
  // means anything.
  onTab: (next: CompanyTab) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  const archivedParagraphId = useId();
  // Why this account takes no changes from this reader (archived, or not
  // theirs to write), said once for the page. The header's verbs, the document
  // upload and the relationship edges all point at this sentence. A rep on a
  // colleague's account is told once, not offered controls the server refuses.
  const verbRefusal = useCompanyVerbRefusal(company);
  // Set only when the paragraph below renders. The raw `useId()` value is
  // always truthy, so passing it always would tell CompanyActionBadges a
  // sentence is drawn. Its fallback would then point `aria-describedby` at an
  // id nothing on the page carries.
  const archivedReasonId = verbRefusal ? archivedParagraphId : undefined;
  // One composer, opened two ways. Anchored on a timeline message, it answers
  // that message. Anchored on a contact, it starts a new one grounded on the
  // account instead of a thread (ADR-0087). Two pieces of state would let two
  // composers open over each other.
  const [composing, setComposing] = useState<ComposeAnchor | null>(null);
  // The header's own Write-email drawer. Separate state from `composing`,
  // which anchors on a message or a contact. The layout treats both the same,
  // because both open into the rail's column.
  const [writingEmail, setWritingEmail] = useState(false);
  // The header's Log-activity/Add-task drawer, lifted here like `writingEmail`.
  // The daily brief's leading card opens the same drawer from its own verb, so
  // the header strip is not the only door into it.
  const [activityDrawer, setActivityDrawer] = useState<ActivityDrawer>(null);
  const [decisionsOpen, setDecisionsOpen] = useState(false);
  const [auditOpen, setAuditOpen] = useState(false);
  // Which open task the Tasks tab has expanded into its detail modal. Keyed by
  // activity id rather than a boolean, because the modal reads that row.
  const [openTaskId, setOpenTaskId, openTaskInTab] = useOpenTaskId(
    company.id,
    tab,
  );
  // Opens a task the daily brief named, from off the Tasks tab. The detail
  // modal mounts only there (CompanyDealsAndTasksTabs). So the card sends the
  // reader there with the task primed, which the tab switch does not wipe.
  const openTaskFromToday = (activityId: string) => {
    openTaskInTab("tasks", activityId);
    onTab("tasks");
  };
  const receipt = useCitedReceipt();
  // Shared by the Tasks tab's row verbs and its detail modal, so both land on
  // one mutation and invalidate the same reads. Those are the account's
  // timeline (the 360 renders it, not an `["activities", …]` query), the
  // workspace-wide queue, and the task's own detail.
  const taskUpdate = useTaskUpdate(taskWriteKeys("company", company.id));
  // The pane answers to its own switch and to nothing else. A composer is a
  // portalled drawer over a scrim, so it takes none of the page's width. An
  // overlay that folded the column would animate the record behind its own
  // backdrop, and leave the pane shut when the reader closed it.
  const details = usePageAside();
  const rail = (
    <CompanyRail
      companyId={company.id}
      company={company}
      view={view}
      // The composite read still in flight, as opposed to failed. An
      // undefined `view` alone cannot tell the rail's sections which of the
      // two holds, and each needs its own drawing.
      loading={loading}
      onTab={onTab}
    />
  );
  const {
    slots,
    showChanges: filterToChanges,
    openEmail,
    setOpenEmail,
  } = useChronologySlots({
    company,
    view,
    loading,
    failed,
    active: tab === "timeline",
  });
  const showChanges = () => {
    onTab("timeline");
    filterToChanges();
  };
  return (
    <div className="record-sheet">
      <RecordView
        back={<WorklistReturnLink />}
        name={company.display_name}
        identity={company.id}
        avatarSrc={company.logo_url}
        // One rung under the record scale. The name stays the largest thing on
        // the page. Beside a work column that opens on the reader's ask, it
        // need not be the size of a masthead.
        scale="compact"
        // Where the account stands, the one control on the name's own line.
        nameBadge={<CompanyNameLine company={company} />}
        // What the account is, the way in, what it is to us and who may read
        // it, as the row under the name.
        pulse={<CompanyMarks company={company} />}
        zone={recordZone}
        // The way in, who holds the account and when its own row was written,
        // as the facts strip every record page carries under its pulse.
        //
        // The prose description reads in the details grid instead. It is the
        // one header item a reader cannot act on, and it has no length bound.
        // On an enriched account it repeats the industry two lines above.
        badges={
          <CompanyIdentityFacts
            company={company}
            view={view}
            loading={loading}
          />
        }
        // The composer opens from a button rather than standing open above the
        // page. A whole form in the header's action strip would push the
        // account's own story below the fold.
        actions={
          <>
            {/* One sentence for the whole strip. Both action groups below refuse
              for the same reason, so the reason belongs to the page. Stated in
              each group, an archived account would say it twice once the menu
              opened. */}
            {verbRefusal && (
              <p className="t-caption" id={archivedParagraphId}>
                {verbRefusal}
              </p>
            )}
            <CompanyHeaderActions
              company={company}
              composerOpen={writingEmail}
              onComposerOpen={setWritingEmail}
              drawer={activityDrawer}
              onDrawer={setActivityDrawer}
              archivedReasonId={archivedReasonId}
            />
            {/* Last in the row, after the verbs it holds the remainder of. A
              menu of everything else reads as the first thing to press when it
              leads the row. */}
            <CompanyActionBadges
              company={company}
              archivedReasonId={archivedReasonId}
              view={view}
              onOpenHistory={() => setAuditOpen(true)}
              onSetUpPartner={() => onTab("partner")}
              onOpenDecisions={
                tab === "overview" ? () => setDecisionsOpen(true) : undefined
              }
            />
          </>
        }
        actionsInline
        // The account's context, beside the work under the tab row. What is
        // true of the account does not belong to the open tab, so the pane
        // stays put when a tab changes.
        aside={rail}
        asideOpen={details.open}
        // The bar that chooses which part of the account to read, across the
        // page above the columns. The details pane opens under it, from the
        // control at its end.
        tabs={tabs}
        // The chronology is the account's story and belongs to the overview.
        // The Partner tab is a form, so it does not repeat it under itself.
        {...slots}
      >
        <CompanyRecordBody
          company={company}
          view={view}
          refusedReasonId={archivedReasonId}
          loading={loading}
          failed={failed}
          tab={tab}
          onTab={onTab}
          t={t}
          receipt={receipt}
          composing={composing}
          onCompose={setComposing}
          onPerform={(action) => {
            // Total over the kinds the server can name. A kind this page
            // cannot perform is a compile error here, never a button that
            // swallows the click.
            switch (action.kind) {
              case "draft_reply":
                if (action.activity_id) {
                  setComposing({ kind: "reply", id: action.activity_id });
                }
                return;
              case "open_deal":
                if (action.deal_id) {
                  navigate({ screen: "deals", id: action.deal_id });
                }
                return;
              case "add_task":
                // Never reached: the advice section writes the step itself,
                // through the POST /tasks the task form uses. The server
                // prepared the body, so this page would only relay it. A
                // surface here would add a second author to a rule's sentence.
                return;
              default:
                unreachableAction(action.kind);
            }
          }}
          decisionsOpen={decisionsOpen}
          onDecisionsOpen={setDecisionsOpen}
          openTaskId={openTaskId}
          onOpenTask={setOpenTaskId}
          onOpenTaskFromToday={openTaskFromToday}
          onLogActivity={() => setActivityDrawer("log")}
          taskUpdate={taskUpdate}
          onOpenHistory={showChanges}
        />
        {/* The email drawer, on the same rule as the audit spine below: it
          belongs to the record. In the Timeline tab's own slot it would
          unmount with that tab and leave its id behind. Returning to Timeline
          could then put it over an already-open dialog. */}
        <OpenEmailDrawer
          activityId={openEmail}
          zone={recordZone}
          onClose={() => setOpenEmail(null)}
        />
        {/* The audit spine, opened from the header's overflow menu. It belongs
          to the record, not to a tab, so it opens over whichever tab is up. */}
        <Modal
          open={auditOpen}
          onClose={() => setAuditOpen(false)}
          labelledBy="co-audit-title"
          intent="drawer-reading"
        >
          <DrawerHead>
            <Heading size="large" id="co-audit-title" className="t-h2">
              {t("record.fullHistory")}
            </Heading>
          </DrawerHead>
          <DrawerBody>
            {/* Mounted only while open: the two history reads behind it are
              the page's most expensive, and only a reader who opens it pays. */}
            {auditOpen && (
              <RecordHistoryTab
                kind="company"
                id={company.id}
                restore={{
                  version: company.version,
                  onRestored: () => invalidateCompany(queryClient, company),
                }}
              />
            )}
          </DrawerBody>
        </Modal>
      </RecordView>
    </div>
  );
}
