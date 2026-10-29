// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import {
  TaskCompleteCheck,
  type useTaskUpdate,
} from "../../components/taskactions";
import { AppLink } from "../../design-system/applink";
import { Badge, Button } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { formatMoneyWhole, formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { PHASE_LABEL } from "../../format/projectphase";
import { useLocale, usePlural, useT } from "../../i18n";
import type { CompanyTab } from "../../lib/companytab";
import { usePipelines } from "../../lib/pipelines.queries";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { recordNamesIn } from "./company360";
import { CompanyContactCard } from "./companycontactcard";
import { LatestCard } from "./companyoverviewlatest";
import {
  economicBuyerMissing,
  otherOpenTasks,
  stageRank,
  strongestContacts,
  taskStanding,
  type WorkRow,
  workInFlight,
} from "./companyoverviewrows";
import "./company360.css";

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];
type TaskUpdate = ReturnType<typeof useTaskUpdate>;
type Section = Company360["sections_omitted"][number];

/**
 * The four cards under How it stands, in the design's two columns. The other
 * open tasks sit over what is in flight, the newest exchanges over who is on
 * file. Each card's link opens the tab that holds the whole list.
 */
export function CompanyOverviewCards({
  view,
  shownTaskId,
  taskUpdate,
  onOpenTab,
  onOpenTask,
  onOpenEmail,
}: Readonly<{
  view: Company360;
  // The task the Next bar already draws, left out of the list under it.
  shownTaskId?: string;
  // The tick's write. Absent draws no tick: a box that cannot be ticked is
  // worse than none.
  taskUpdate?: TaskUpdate;
  onOpenTab: (tab: CompanyTab) => void;
  onOpenTask: (activityId: string) => void;
  onOpenEmail: (activityId: string) => void;
}>) {
  return (
    <div className="co-overview-grid">
      <div className="co-overview-column">
        <AlsoOnYourList
          view={view}
          shownTaskId={shownTaskId}
          taskUpdate={taskUpdate}
          onOpenTab={onOpenTab}
          onOpenTask={onOpenTask}
        />
        <DealsAndProjects view={view} onOpenTab={onOpenTab} />
      </div>
      <div className="co-overview-column">
        <LatestCard
          view={view}
          onOpenTab={onOpenTab}
          onOpenEmail={onOpenEmail}
        />
        <ContactsCard view={view} onOpenTab={onOpenTab} />
      </div>
    </div>
  );
}

// A section this reader may not see says so instead of reading as empty.
function emptyWords(
  view: Company360,
  sections: readonly Section[],
): "co.section.restricted" | undefined {
  return sections.every((section) => view.sections_omitted.includes(section))
    ? "co.section.restricted"
    : undefined;
}

function AlsoOnYourList({
  view,
  shownTaskId,
  taskUpdate,
  onOpenTab,
  onOpenTask,
}: Readonly<{
  view: Company360;
  shownTaskId?: string;
  taskUpdate?: TaskUpdate;
  onOpenTab: (tab: CompanyTab) => void;
  onOpenTask: (activityId: string) => void;
}>) {
  const t = useT();
  const steps = otherOpenTasks(view, shownTaskId);
  return (
    <OverviewCard
      title={t("co.overview.alsoOnList")}
      action={
        <Button variant="link" onClick={() => onOpenTab("tasks")}>
          {t("record.allTasks")}
        </Button>
      }
      empty={t(
        emptyWords(view, ["next_steps"]) ??
          (shownTaskId ? "co.overview.nothingOpen" : "co.next.empty"),
      )}
    >
      {steps.length > 0 && (
        <OverviewList>
          {steps.map((step) => (
            <TaskRow
              key={step.activity_id}
              step={step}
              view={view}
              taskUpdate={taskUpdate}
              onOpenTask={onOpenTask}
            />
          ))}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

function TaskRow({
  step,
  view,
  taskUpdate,
  onOpenTask,
}: Readonly<{
  step: NextStep;
  view: Company360;
  taskUpdate?: TaskUpdate;
  onOpenTask: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const nameOf = recordNamesIn(view);
  const linked =
    (step.linked_deal_id && nameOf("deal", step.linked_deal_id)) ||
    (step.linked_contact_id && nameOf("contact", step.linked_contact_id)) ||
    undefined;
  const sub = [
    step.due_at &&
      t("co.next.due", { when: formatDayMonth(step.due_at, locale, zone) }),
    linked && t("co.overview.for", { name: linked }),
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <OverviewRow
      lead={
        taskUpdate && (
          <TaskCompleteCheck
            activityId={step.activity_id}
            version={step.version}
            update={taskUpdate}
          />
        )
      }
      title={
        <button
          type="button"
          className="co-rowlink"
          onClick={() => onOpenTask(step.activity_id)}
        >
          {step.subject}
        </button>
      }
      sub={sub || undefined}
      right={<TaskStandingMark step={step} asOf={view.as_of} />}
    />
  );
}

/** Where a task stands, as the right end of its row: a pill while it is late
 *  or close, the day itself further out, "No date" when it has none. */
export function TaskStandingMark({
  step,
  asOf,
}: Readonly<{ step: NextStep; asOf: string }>): ReactNode {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const standing = taskStanding(step, asOf, zone);
  switch (standing.kind) {
    case "late":
      return (
        <Badge tone="danger">
          {standing.days === undefined
            ? t("co.next.overdue")
            : plural("co.next.daysLate", standing.days, {
                days: formatNumber(standing.days, locale),
              })}
        </Badge>
      );
    case "today":
      return <Badge tone="warning">{t("co.next.dueToday")}</Badge>;
    case "soon":
      return (
        <Badge tone="warning">
          {plural("co.next.inDays", standing.days, {
            days: formatNumber(standing.days, locale),
          })}
        </Badge>
      );
    case "later":
      return formatDayMonth(standing.due, locale, zone);
    case "undated":
      return t("co.overview.noDate");
  }
}

function DealsAndProjects({
  view,
  onOpenTab,
}: Readonly<{ view: Company360; onOpenTab: (tab: CompanyTab) => void }>) {
  const t = useT();
  const rows = workInFlight(view);
  return (
    <OverviewCard
      title={t("tab.dealsProjects")}
      action={
        <Button variant="link" onClick={() => onOpenTab("deals")}>
          {t("co.overview.allDeals")}
        </Button>
      }
      empty={t(emptyWords(view, ["deals", "projects"]) ?? "co.overview.noWork")}
    >
      {rows.length > 0 && (
        <OverviewList>
          {rows.map((row) =>
            row.kind === "deal" ? (
              <DealItem key={`deal-${row.deal.deal_id}`} deal={row.deal} />
            ) : (
              <ProjectItem
                key={`project-${row.project.project_id}`}
                project={row.project}
              />
            ),
          )}
        </OverviewList>
      )}
    </OverviewCard>
  );
}

function DealItem({
  deal,
}: Readonly<{ deal: Extract<WorkRow, { kind: "deal" }>["deal"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const pipelines = usePipelines();
  const rank = stageRank(deal.stage_id, pipelines.data);
  const sub = [
    deal.stage_name,
    rank &&
      t("co.overview.stageOf", {
        rank: formatNumber(rank.rank, locale),
        of: formatNumber(rank.of, locale),
      }),
    deal.expected_close_date &&
      t("co.overview.closes", {
        date: formatDayMonth(deal.expected_close_date, locale, zone),
      }),
  ]
    .filter(Boolean)
    .join(" · ");
  const amount = deal.amount;
  return (
    <OverviewRow
      title={
        <AppLink
          className="co-rowlink"
          href={routeHash({ screen: "deals", id: deal.deal_id })}
        >
          {deal.name}
        </AppLink>
      }
      sub={sub || undefined}
      right={
        amount?.amount_minor != null && amount.currency
          ? formatMoneyWhole(amount.amount_minor, amount.currency, locale)
          : undefined
      }
      figure
    />
  );
}

function ProjectItem({
  project,
}: Readonly<{ project: Extract<WorkRow, { kind: "project" }>["project"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const sub = [
    t("co.overview.project"),
    t(PHASE_LABEL[project.phase]),
    project.target_end_date &&
      t("co.overview.ends", {
        date: formatDayMonth(project.target_end_date, locale, zone),
      }),
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <OverviewRow
      title={
        <AppLink
          className="co-rowlink"
          href={routeHash({ screen: "projects", id: project.project_id })}
        >
          {project.name}
        </AppLink>
      }
      sub={sub}
    />
  );
}

function ContactsCard({
  view,
  onOpenTab,
}: Readonly<{ view: Company360; onOpenTab: (tab: CompanyTab) => void }>) {
  const t = useT();
  const contacts = strongestContacts(view);
  return (
    <OverviewCard
      title={t("tab.contacts")}
      action={
        <Button variant="link" onClick={() => onOpenTab("contacts")}>
          {t("co.rail.contacts.add")}
        </Button>
      }
      empty={t(emptyWords(view, ["contacts"]) ?? "co.rail.contacts.empty")}
      foot={
        economicBuyerMissing(view)
          ? t("co.overview.noEconomicBuyer")
          : undefined
      }
      footTone="warn"
    >
      {contacts.length > 0 &&
        contacts.map((contact) => (
          <CompanyContactCard
            key={contact.contact_id}
            contact={contact}
            companyName={view.company.display_name}
          />
        ))}
    </OverviewCard>
  );
}
