import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { sectionState } from "../../design-system/surfacestate";
import { calendarDaysUntil } from "../../format/daysuntil";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { daysLate } from "./companynextbar";
import "./companytabs.css";
import "./company360.css";

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

type Cells = {
  t: ReturnType<typeof useT>;
  plural: ReturnType<typeof usePlural>;
  locale: Locale;
  zone: string;
  asOf: string;
  onOpenTask?: (step: NextStep) => void;
  renderTick?: (step: NextStep) => ReactNode;
  renderAction?: (step: NextStep) => ReactNode;
};

/**
 * CompanyTaskTable is the Tasks tab's list of open tasks as a table. Each row
 * has the tick and the task, its due day, its owner and where it stands
 * against the 360's own `as_of`. A withheld section is the caller's to word.
 */
export function CompanyTaskTable({
  view,
  onOpenTask,
  renderTick,
  renderAction,
}: Readonly<{
  view: Company360;
  onOpenTask?: (step: NextStep) => void;
  // The completion tick before the task's name. Absent (a read-only account)
  // draws none: a box that cannot be ticked is worse than no box.
  renderTick?: (step: NextStep) => ReactNode;
  renderAction?: (step: NextStep) => ReactNode;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const steps = view.next_steps?.data ?? [];
  const state = sectionState(
    view,
    "next_steps",
    Boolean(view.next_steps),
    steps.length,
  );
  const cells: Cells = {
    t,
    plural,
    locale,
    zone,
    asOf: view.as_of,
    onOpenTask,
    renderTick,
    renderAction,
  };
  return (
    <Panel title={t("co.next.title")}>
      {state === "ready" && (
        <DataTable
          bleed
          fold
          label={t("co.next.title")}
          columns={taskColumns(cells)}
          rows={steps}
          rowKey={(step) => step.activity_id}
        />
      )}
      {state === "empty" && (
        <PanelBody>
          <p className="surfacestate-empty">{t("co.next.empty")}</p>
        </PanelBody>
      )}
      {state === "unavailable" && (
        <PanelBody>
          <p className="surfacestate-withheld">{t("co.section.unavailable")}</p>
        </PanelBody>
      )}
    </Panel>
  );
}

function taskColumns(cells: Cells): DataTableColumn<NextStep>[] {
  const { t, locale, zone, renderAction } = cells;
  const columns: DataTableColumn<NextStep>[] = [
    {
      key: "task",
      header: t("co.next.col.task"),
      grow: true,
      render: (step) => <TaskCell step={step} cells={cells} />,
    },
    {
      key: "due",
      header: t("co.next.col.due"),
      render: (step) =>
        step.due_at ? formatDayMonth(step.due_at, locale, zone) : "",
    },
    {
      key: "owner",
      header: t("co.next.col.owner"),
      render: (step) =>
        step.assignee_id ? <EntityRef kind="user" id={step.assignee_id} /> : "",
    },
    {
      key: "state",
      header: t("co.next.col.state"),
      fold: "end",
      render: (step) => <TaskStanding step={step} cells={cells} />,
    },
  ];
  if (renderAction) {
    columns.push({
      key: "verbs",
      header: t("co.next.col.verbs"),
      headerHidden: true,
      fold: "end",
      render: (step) => (
        <span className="cell-actions">{renderAction(step)}</span>
      ),
    });
  }
  return columns;
}

function TaskCell({ step, cells }: Readonly<{ step: NextStep; cells: Cells }>) {
  const { onOpenTask, renderTick } = cells;
  return (
    <span className="co-tasktable-task">
      {renderTick?.(step)}
      <span className="co-tasktable-name">
        {onOpenTask ? (
          <button
            type="button"
            className="co-rowlink"
            onClick={() => onOpenTask(step)}
          >
            {step.subject}
          </button>
        ) : (
          <span>{step.subject}</span>
        )}
        {(step.linked_deal_id || step.linked_contact_id) && (
          <span className="co-row-meta t-caption">
            {step.linked_deal_id && (
              <EntityRef kind="deal" id={step.linked_deal_id} />
            )}
            {step.linked_contact_id && (
              <EntityRef kind="contact" id={step.linked_contact_id} />
            )}
          </span>
        )}
      </span>
    </span>
  );
}

// Where a task stands on the 360's own day: late by whole days, due in so
// many days, or undated. Counted from `as_of`, never the browser clock.
function TaskStanding({
  step,
  cells,
}: Readonly<{ step: NextStep; cells: Cells }>) {
  const { t, plural, locale, zone, asOf } = cells;
  if (!step.due_at) {
    return <Badge>{t("co.next.undated")}</Badge>;
  }
  const late = daysLate(step, asOf, zone);
  if (step.overdue) {
    return (
      <Badge tone="danger">
        {late === undefined
          ? t("co.next.overdue")
          : plural("co.next.daysLate", late, {
              days: formatNumber(late, locale),
            })}
      </Badge>
    );
  }
  const ahead = calendarDaysUntil(step.due_at, zone, new Date(asOf));
  return (
    <Badge>
      {ahead <= 0
        ? t("co.next.dueToday")
        : plural("co.next.inDays", ahead, {
            days: formatNumber(ahead, locale),
          })}
    </Badge>
  );
}
