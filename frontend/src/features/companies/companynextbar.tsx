import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { RecordNextBar } from "../../components/latestinbound";
import { taskLine, useTaskHolder } from "../../components/taskassignee";
import { formatDueCompact } from "../../format/format.dates";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import { type LatePromise, latePromise } from "../../lib/latepromise";
import { tasksPeek } from "../../lib/standingpeeks";
import { taskDetail } from "../../lib/taskdetail";
import { useViewerZone } from "../../lib/viewerzone";
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

/**
 * How late the bar's task is, or nothing when it is on time. "How it stands"
 * reads it too, so the card's sentence and the bar's badge give one count.
 */
export function nextBarLateness(
  view: Company360,
  zone: string,
  words: Words,
): LatePromise | undefined {
  const task = nextBarTask(view);
  return task?.due_at
    ? latePromise(task.due_at, view.as_of, zone, "task", words)
    : undefined;
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
  const zone = useViewerZone();
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
  const said = nextBarLateness(view, zone, { t, plural, locale });
  const lateTag = said && { label: said.badge, tone: "danger" as const };
  // The bar's task is the account's, not the reader's: its line names who holds it.
  const holder = useTaskHolder(task?.assignee_id, task?.assignee_name);
  const heldBy = (rest: ReactNode) => (task ? taskLine([holder, rest]) : rest);
  const count = otherTasks(view, task);
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
      <RecordNextBar
        // A company moment is an owed promise, which outranks a new mail.
        urgent
        about={moment.evidence.map((item) => item.id)}
        // The task the moment cites is the thing to do; the headline is the
        // server's sentence about it, kept for a moment that cites none.
        title={task?.subject ?? moment.headline}
        late={lateTag}
        detail={task && taskDetail(task)}
        sub={heldBy(said?.sentence ?? moment.why_now)}
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
    <RecordNextBar
      // The Overview's "Last contact" tile already shows the last exchange.
      lastExchange={false}
      title={task?.subject}
      late={lateTag}
      detail={task && taskDetail(task)}
      sub={heldBy(
        task?.due_at
          ? t("co.next.due", {
              when: formatDueCompact(
                task.due_at,
                task.due_has_time,
                locale,
                zone,
              ),
            })
          : undefined,
      )}
      action={task ? fallback : undefined}
      more={more}
    />
  );
}
