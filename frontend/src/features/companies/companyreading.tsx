// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { useTaskUpdate } from "../../components/taskactions";
import { CompanyHowItStands } from "./companyhowitstands";
import { CompanyNextBar, nextBarTask } from "./companynextbar";
import { CompanyOverviewCards } from "./companyoverviewcards";
import type { CompanyTab } from "../../lib/companytab";

type Company = components["schemas"]["Company"];
type Company360View = components["schemas"]["Company360"];

export type CompanyOverviewProps = Readonly<{
  company: Company;
  view?: Company360View;
  // The composite read's own pending flag (see CompanyRecordBody).
  loading: boolean;
  failed: boolean;
  onOpenRecord: (entityType: string, entityId: string) => void;
  onOpenEmail: (activityId: string) => void;
  onOpenTasks: () => void;
  // The Next bar's own fallback verbs: a named task's detail modal, and the
  // header's Log-activity drawer.
  onOpenTask: (activityId: string) => void;
  onLogActivity: () => void;
  onDraftTo: (contactId: string) => void;
  onOpenTab: (tab: CompanyTab) => void;
  // The tick on the Overview's task list, the same write the Tasks tab uses.
  taskUpdate?: ReturnType<typeof useTaskUpdate>;
}>;

// The Overview's body: what to do next, how the account stands, then the
// cards it is read from. Every part reads the 360 or the money summary, and
// the server refuses both for an archived account, so the page leaves it out.
export function CompanyReading({
  company,
  view,
  loading,
  failed,
  onOpenRecord,
  onOpenEmail,
  onOpenTasks,
  onOpenTask,
  onLogActivity,
  onDraftTo,
  onOpenTab,
  taskUpdate,
}: CompanyOverviewProps) {
  const ready = !loading && !failed ? view : undefined;
  return (
    <>
      {ready && (
        <CompanyNextBar
          view={ready}
          onOpenRecord={onOpenRecord}
          onOpenTask={onOpenTask}
          onOpenTasks={onOpenTasks}
          onDraftTo={onDraftTo}
          onLogActivity={onLogActivity}
        />
      )}
      <CompanyHowItStands
        companyId={company.id}
        view={view}
        loading={loading}
        failed={failed}
        onOpenTab={onOpenTab}
      />
      {ready && (
        <CompanyOverviewCards
          view={ready}
          shownTaskId={nextBarTask(ready)?.activity_id}
          taskUpdate={taskUpdate}
          onOpenTab={onOpenTab}
          onOpenTask={onOpenTask}
          onOpenEmail={onOpenEmail}
        />
      )}
    </>
  );
}
