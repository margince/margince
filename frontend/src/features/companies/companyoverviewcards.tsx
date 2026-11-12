// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { OpenTaskRow } from "../../components/opentaskrow";
import type { useTaskUpdate } from "../../components/taskactions";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import { RecordCardList } from "../../design-system/recordcard";
import { formatMoneyWhole, formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { PHASE_LABEL } from "../../format/projectphase";
import { useLocale, useT } from "../../i18n";
import type { CompanyTab } from "../../lib/companytab";
import { useDisabledCoreFeatures } from "../../lib/corefeatures";
import { labelFor } from "../../lib/labelfor";
import { usePipelines } from "../../lib/pipelines.queries";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { recordNamesIn } from "./company360";
import { CompanyContactCard } from "./companycontactcard";
import { LatestCard } from "./companyoverviewlatest";
import {
  economicBuyerMissing,
  otherOpenTasks,
  stageRank,
  strongestContacts,
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
        <OtherOpenTasks
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

function OtherOpenTasks({
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
      title={t("co.overview.otherOpen")}
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
  const nameOf = recordNamesIn(view);
  const linked =
    (step.linked_deal_id && nameOf("deal", step.linked_deal_id)) ||
    (step.linked_contact_id && nameOf("contact", step.linked_contact_id)) ||
    undefined;
  return (
    <OpenTaskRow
      // The company read carries no per-row `writable`, so every row keeps
      // its tick.
      task={{ ...step, id: step.activity_id, writable: true }}
      asOf={view.as_of}
      taskUpdate={taskUpdate}
      forName={linked}
      title={
        <button
          type="button"
          className="co-rowlink"
          onClick={() => onOpenTask(step.activity_id)}
        >
          {step.subject}
        </button>
      }
    />
  );
}

function DealsAndProjects({
  view,
  onOpenTab,
}: Readonly<{ view: Company360; onOpenTab: (tab: CompanyTab) => void }>) {
  const t = useT();
  const disabled = useDisabledCoreFeatures();
  const rows = workInFlight(view).filter(
    (row) => !disabled.includes(row.kind === "deal" ? "deals" : "projects"),
  );
  if (disabled.includes("deals") && disabled.includes("projects")) {
    return null;
  }
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
  const zone = useViewerZone();
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
  const zone = useViewerZone();
  const sub = [
    t("co.overview.project"),
    t(labelFor(PHASE_LABEL, project.phase)),
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
  const disabled = useDisabledCoreFeatures();
  const contacts = strongestContacts(view);
  if (disabled.includes("contacts")) {
    return null;
  }
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
      {contacts.length > 0 && (
        <RecordCardList>
          {contacts.map((contact) => (
            <li key={contact.contact_id}>
              <CompanyContactCard
                contact={contact}
                companyName={view.company.display_name}
              />
            </li>
          ))}
        </RecordCardList>
      )}
    </OverviewCard>
  );
}
