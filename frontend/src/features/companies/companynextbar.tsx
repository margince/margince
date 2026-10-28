import type { components } from "../../api/schema";
import { NextBar } from "../../design-system/nextbar";
import { formatNumber } from "../../format/format";
import { calendarDaysBetween, formatDayMonth } from "../../format/format.dates";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { tasksPeek } from "../../lib/standingpeeks";
import { taskDetail } from "../../lib/taskdetail";
import { useCompanySetAside } from "./companysetaside";
import {
  MayBeDoneVerb,
  momentBasis,
  momentDefer,
  momentFallbackVerb,
  momentVerb,
} from "./companytodayverbs";

type Company360 = components["schemas"]["Company360"];
type NextStep = NonNullable<Company360["next_steps"]>["data"][number];

/**
 * The open task a moment's promise rests on, when the server named one. Only
 * that task dates the lateness: an overdue row the moment does not cite is a
 * different promise.
 */
export function promisedTask(view: Company360): NextStep | undefined {
  const moment = view.moment;
  if (!moment || moment.rule === "nothing_needed") {
    return undefined;
  }
  const cited = new Set(
    moment.evidence
      .filter((item) => item.type === "task" && item.id)
      .map((item) => item.id),
  );
  return view.next_steps?.data.find((step) => cited.has(step.activity_id));
}

/** The task the bar draws: the moment's own when it names something owed,
 *  else the first open one. */
export function nextBarTask(view: Company360): NextStep | undefined {
  return view.moment && view.moment.rule !== "nothing_needed"
    ? promisedTask(view)
    : view.next_steps?.data[0];
}

/** Whole days a task is past its date at `as_of`, or nothing when it is not. */
export function daysLate(
  step: NextStep,
  asOf: string,
  zone: string,
): number | undefined {
  if (!step.overdue || !step.due_at) {
    return undefined;
  }
  const days = calendarDaysBetween(new Date(step.due_at), new Date(asOf), zone);
  return days > 0 ? days : undefined;
}

/**
 * How many other open tasks stand behind the bar. Only a whole page can be
 * counted; a cut one would understate the list.
 */
export function otherTasks(
  view: Company360,
  shown: NextStep | undefined,
): number | undefined {
  const steps = view.next_steps;
  if (!steps || steps.page.has_more) {
    return undefined;
  }
  return steps.data.length - (shown ? 1 : 0);
}

/**
 * The one thing to do next on the account: the server's moment when it names
 * something owed, else the first open task. The verbs are the ones the moment
 * card always had. The bar opens what the server named or, failing that, a
 * surface the page owns.
 */
export function CompanyNextBar({
  view,
  onOpenRecord,
  onOpenTask,
  onOpenTasks,
  onDraftTo,
  onLogActivity,
}: Readonly<{
  view: Company360;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenTask?: (activityId: string) => void;
  onOpenTasks: () => void;
  onDraftTo?: (contactId: string) => void;
  onLogActivity?: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const { deferOf } = useCompanySetAside(view.company.id);
  const fallback = momentFallbackVerb({
    view,
    t,
    onOpenTask,
    onDraftTo,
    onLogActivity,
  });
  const moment =
    view.moment && view.moment.rule !== "nothing_needed"
      ? view.moment
      : undefined;
  const task = nextBarTask(view);
  const late = task && daysLate(task, view.as_of, zone);
  const count = otherTasks(view, task);
  const lateTag =
    late === undefined
      ? undefined
      : {
          label: plural("co.next.daysLate", late, {
            days: formatNumber(late, locale),
          }),
          tone: "danger" as const,
        };
  const others = (view.next_steps?.data ?? []).filter(
    (step) => step.activity_id !== task?.activity_id,
  );
  const more =
    count === undefined
      ? undefined
      : {
          count,
          onOpen: onOpenTasks,
          peek: tasksPeek(
            others.map((step) => ({
              id: step.activity_id,
              subject: step.subject,
              dueAt: step.due_at,
              overdue: step.overdue,
            })),
            { t, plural, locale, zone },
            count,
            onOpenTask,
          ),
        };
  if (moment) {
    const defer = momentDefer(moment, deferOf);
    return (
      <NextBar
        // The task the moment cites is the thing to do; the headline is the
        // server's sentence about it, kept for a moment that cites none.
        title={task?.subject ?? moment.headline}
        late={lateTag}
        detail={task && taskDetail(task)}
        sub={moment.why_now}
        insight={momentBasis(moment)}
        action={
          moment.may_be_done ? (
            <MayBeDoneVerb moment={moment} companyId={view.company.id} />
          ) : (
            momentVerb({ moment, onOpenRecord, fallback })
          )
        }
        snooze={defer && { onSnooze: defer.onDefer, pending: defer.pending }}
        more={more}
      />
    );
  }
  return (
    <NextBar
      title={task?.subject}
      late={lateTag}
      detail={task && taskDetail(task)}
      sub={
        task?.due_at
          ? t("co.next.due", {
              when: formatDayMonth(task.due_at, locale, zone),
            })
          : undefined
      }
      action={task ? fallback : undefined}
      more={more}
    />
  );
}
