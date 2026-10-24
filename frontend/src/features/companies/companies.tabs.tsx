import type { ReactNode } from "react";
import {
  TaskCompleteCheck,
  TaskQuickActions,
  type useTaskUpdate,
} from "../../components/taskactions";
import { Card, EmptyState, Skeleton } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { sectionState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import type { OpenReceipt } from "../records/record360";
import { TaskDetailModal } from "../worklist/taskdetailmodal";
import type { Company, Company360View } from "./companies";
import {
  DealsCard,
  ProposedNextSteps,
  type SuggestionAction,
} from "./company360";
import { NewDealAction } from "./companyactions";
import { CompanyContractState, CompanyLastOffer } from "./companycommercial";
import { CompanyProjectsPanel } from "./companyprojects";
import type { CompanyTab } from "../../lib/companytab";
import { CompanyTasksFinds } from "./companytasksfinds";
import { CompanyTaskTable } from "./companytaskstable";
import "./company360.css";
import "./companytabs.css";

// CompanyDealsAndTasksTabs holds both new tab bodies plus the modal either
// can open. It lives apart from CompanyPage to keep that render legible: the
// two tabs are mutually exclusive and share nothing but the record.
export function CompanyDealsAndTasksTabs({
  tab,
  company,
  view,
  loading,
  failed,
  openTaskId,
  onOpenTask,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPrepareMeeting,
  onDraftTo,
  onPerform,
  taskUpdate,
}: Readonly<{
  tab: CompanyTab;
  company: Company;
  view?: Company360View;
  // The composite read's own states, which the projects block needs to tell a
  // wait from a failure.
  loading: boolean;
  failed: boolean;
  openTaskId: string | null;
  onOpenTask: (activityId: string | null) => void;
  // Where a recommended step's citations lead. They use the page's own, so a
  // chip on the Tasks tab lands where the same chip on the overview does.
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  onOpenEmail: (activityId: string) => void;
  // The verbs the Tasks tab's finds perform: the meeting brief, the
  // composer, and a suggestion's own action.
  onPrepareMeeting: (activityId: string) => void;
  onDraftTo: (contactId: string) => void;
  onPerform: (action: SuggestionAction) => void;
  taskUpdate: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  return (
    <>
      {tab === "deals" && !company.archived_at && (
        // The tab body's own stack: two bare sibling panels would read as one
        // card with a rule through it.
        <div className="co-tab-stack">
          {/* Beside the deals card, not inside it. The card's `extra` slot only
              renders when the deals section itself is readable. A reader with
              the contract grant and not the deal grant would then never see
              what the account is under contract for.
              In its own Panel: the block renders PanelBody rows, and bare over
              the deals card it reads as a line that fell out of one.
              The Panel is drawn only when the reader holds the contract grant
              (the same `contracts` slice the block reads) and the account has
              an agreement. A reader without the grant gets no empty card. An
              account with nothing on record gets none either, because the deals
              card under it already implies "no contract". */}
          {view?.state_strip?.contracts &&
            view.state_strip.contracts.active_count > 0 && (
              <Panel title={t("co.commercial.title")}>
                <CompanyContractState view={view} />
              </Panel>
            )}
          <CompanyDealsTab company={company} view={view} failed={failed} />
          {/* The deliveries this account is part of, as their own block under
              the pipeline. The tab reads "Deals & projects" because both are
              the account's work in flight. A reader who came for one finds the
              other without a second tab to try. */}
          <CompanyProjectsPanel
            companyId={company.id}
            view={view}
            loading={loading}
            failed={failed}
          />
        </div>
      )}
      {tab === "tasks" && !company.archived_at && (
        <CompanyTasksTab
          companyId={company.id}
          view={view}
          failed={failed}
          onOpenTask={onOpenTask}
          onOpenRecord={onOpenRecord}
          onOpenReceipt={onOpenReceipt}
          update={taskUpdate}
          finds={
            view && (
              <CompanyTasksFinds
                companyId={company.id}
                view={view}
                onPrepareMeeting={onPrepareMeeting}
                onDraftTo={onDraftTo}
                onOpenRecord={onOpenRecord}
                onOpenReceipt={onOpenReceipt}
                onOpenEmail={onOpenEmail}
                onPerform={onPerform}
              />
            )
          }
        />
      )}
      {tab === "tasks" && !company.archived_at && openTaskId && (
        <TaskDetailModal
          activityId={openTaskId}
          readOnly={false}
          onClose={() => onOpenTask(null)}
          update={taskUpdate}
        />
      )}
    </>
  );
}

// CompanyDealsTab: the pipeline plus the last commercial exchange, both cited
// evidence-or-omit off the composite read like every other business card.
// It holds the same loading/failed split CompanyBusinessGrid does. A card with
// no view says "could not load" or "not yet", and only the caller knows which.
//
// One Panel: the last offer reads the same open deals the pipeline list shows.
// So it renders in `DealsCard`'s `extra` slot, not as a second card repeating
// "this account's deals" under another heading.
function CompanyDealsTab({
  company,
  view,
  failed,
}: Readonly<{
  company: Company;
  view?: Company360View;
  failed: boolean;
}>) {
  if (!view && !failed) {
    return (
      <Card className="co-card">
        <Skeleton width="100%" height={96} />
      </Card>
    );
  }
  return (
    <DealsCard
      view={view}
      actions={
        <NewDealAction
          companyId={company.id}
          companyName={company.display_name}
        />
      }
      extra={<CompanyLastOffer view={view} />}
    />
  );
}

// CompanyTasksTab: the account's open tasks, with the tick-to-complete verb
// the work queue offers (taskactions.tsx). It is one mutation, so a task
// finished here is finished on the queue too.
//
// A withheld section is caught here first: the table leaves it to its caller,
// and a blank tab body is what the four-states rule forbids.
function CompanyTasksTab({
  companyId,
  view,
  failed,
  onOpenTask,
  onOpenRecord,
  onOpenReceipt,
  update,
  finds,
}: Readonly<{
  companyId: string;
  view?: Company360View;
  failed: boolean;
  onOpenTask: (activityId: string) => void;
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt;
  update: ReturnType<typeof useTaskUpdate>;
  // What Margince found beyond the proposed steps, under them.
  finds?: ReactNode;
}>) {
  const t = useT();
  if (!view && !failed) {
    return (
      <Card className="co-card">
        <Skeleton width="100%" height={96} />
      </Card>
    );
  }
  if (!view) {
    return <EmptyState>{t("co.partial")}</EmptyState>;
  }
  const steps = view.next_steps?.data ?? [];
  if (
    sectionState(view, "next_steps", Boolean(view.next_steps), steps.length) ===
    "withheld"
  ) {
    // Same chrome as every other state of this tab. A bare refusal plate names
    // no section, and the facts strip above says the same sentence about its
    // own withheld halves. Unheaded, a reader cannot tell which is refused.
    return (
      <Panel title={t("co.next.title")}>
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
        </PanelBody>
      </Panel>
    );
  }
  // The list first, then the advice that would become a task. Each is its own
  // card, so the recommended steps never read as rows already on the list.
  return (
    <div className="co-tab-stack">
      <CompanyTaskTable
        view={view}
        onOpenTask={(step) => onOpenTask(step.activity_id)}
        renderTick={(step) => (
          <TaskCompleteCheck
            activityId={step.activity_id}
            version={step.version}
            update={update}
          />
        )}
        renderAction={(step) => (
          <TaskQuickActions
            activityId={step.activity_id}
            version={step.version}
            dueAt={step.due_at}
            update={update}
            showComplete={false}
          />
        )}
      />
      <ProposedNextSteps
        companyId={companyId}
        view={view}
        onOpenRecord={onOpenRecord}
        onOpenReceipt={onOpenReceipt}
      />
      {finds}
    </div>
  );
}
