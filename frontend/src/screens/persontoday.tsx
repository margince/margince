import {
  CalendarDays,
  CheckSquare,
  FileText,
  Search,
  Send,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Button } from "../design-system/atoms";
import { formatDate, formatNumber } from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useRoster } from "./entityref";
import { interactionIcon } from "./interactionchrome";
import {
  CallCard,
  FoundMove,
  type Grounding,
  RecordSpine,
  type SpineSource,
  type StandingTone,
  TodayPanel,
  TodoRow,
} from "./record360";

// "Today with {first name}" (concept §5.5, ADR-0096 D2), in the two cards
// every record page reads in.
//
// THE CALL: the rule the server's ladder picked, as the standing; `why_now` as
// the sentence it rests on; the records behind it one disclosure away; and
// under them the contact's own thread — what was said, the silence since, what
// is dated ahead. THE DAY'S WORK: the moment's headline as the ask, with the
// verb that performs it and the evidence under it; and beneath, the open tasks
// already on somebody's list.
//
// ONE moment, chosen server-side by the fixed ladder. The client renders what
// it is given and computes nothing about WHICH moment: a page that picked its
// own headline from date comparisons would drift from every other client
// showing the same record, which is the drift the rule/version stamp exists
// to make impossible. What the client does compute is arithmetic between two
// dates it was handed — the thread's gap, a task's lateness — which is the
// same arithmetic on every record.
//
// The action is a TYPED descriptor, so this renders only buttons whose
// destination the server named. An action with no destination still renders —
// some are their own destination — but one this client cannot route is not
// invented into a button that 404s.

type Person360 = components["schemas"]["Person360"];
type PersonMoment = components["schemas"]["PersonMoment"];
type PersonMomentAction = components["schemas"]["PersonMomentAction"];
type Activity = components["schemas"]["Activity"];

// The rule that fired, in one word over the sentence it produced.
//
// The server picks exactly one rule from a fixed ladder, so the word is the
// server's judgement rather than this page's reading of a headline. Named
// rather than left implicit: "Gone quiet" and "Promise overdue" lead to
// different moves, and a reader who sees only the sentence has to infer which
// kind of thing they are looking at.
export const MOMENT_RULE_LABEL = {
  meeting_prep: "person.moment.rule.meeting_prep",
  re_engaged: "person.moment.rule.re_engaged",
  job_change: "person.moment.rule.job_change",
  overdue_promise: "person.moment.rule.overdue_promise",
  gone_quiet: "person.moment.rule.gone_quiet",
  open_promise: "person.moment.rule.open_promise",
  role_change: "person.moment.rule.role_change",
  public_signal: "person.moment.rule.public_signal",
  missing_next_step: "person.moment.rule.missing_next_step",
  thin_relationship: "person.moment.rule.thin_relationship",
  nothing_needed: "person.moment.rule.nothing_needed",
} as const satisfies Record<PersonMoment["rule"], MessageKey>;

export const MOMENT_EVIDENCE_LABEL = {
  activity: "person.moment.evidence.activity",
  task: "person.moment.evidence.task",
  relationship_change: "person.moment.evidence.relationship_change",
} as const satisfies Record<
  components["schemas"]["PersonMomentEvidence"]["type"],
  MessageKey
>;

// The standing's colour. Two rules mean somebody is being kept waiting and
// read as warnings; the quiet success state reads as settled rather than as
// something nobody has judged. Everything else is a live thread — a fact about
// the relationship that wants a move rather than a verdict on it.
export function standingTone(rule: PersonMoment["rule"]): StandingTone {
  if (isLate(rule)) {
    return "warn";
  }
  return rule === "nothing_needed" ? "calm" : "accent";
}

// The two rules that mean somebody is being kept waiting. A promise past its
// date is late whether it was read out of an email or filed as a task — one
// rung covers both — while a promise not yet due is a live thread, not a
// warning.
export function isLate(rule: PersonMoment["rule"]): boolean {
  return rule === "gone_quiet" || rule === "overdue_promise";
}

export function PersonToday({
  moment,
  name,
  view,
  onAction,
  onOpenTasks,
}: Readonly<{
  // Absent when the caller lacks a grant the rule needs — the server names the
  // section in `sections_omitted` — and the page still opens on the call and
  // the day's work: a reading that is withheld says so where the reading goes,
  // it does not disappear and leave the reader wondering which shape this
  // record page has.
  moment?: PersonMoment;
  // The record's full name, for the card's head.
  name: string;
  // The record the thread and the open tasks are read from. The same read the
  // moment arrived on, so the thread cannot disagree with the call above it.
  view: Person360;
  onAction: (action: PersonMomentAction) => void;
  // Where the day's work sends a reader for the whole task list.
  onOpenTasks?: () => void;
}>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const taskRows = useOpenTaskRows(view);
  if (!moment) {
    return (
      <>
        <CallCard
          name={name}
          standing={{ label: t("record.notShown"), tone: "unknown" }}
        >
          <RecordSpine
            source={spineSourceOf(view)}
            commercial={{ next_close_on: view.commercial?.deal?.close_date }}
          />
        </CallCard>
        <TodayPanel onOpenTasks={onOpenTasks}>{taskRows}</TodayPanel>
      </>
    );
  }
  // What the moment rests on, in the shape every claim on a record states it:
  // the label a reader can act on, and the kind of record it was read from.
  // Same disclosure as the account's call, because it is the same promise —
  // nothing a machine says here is unsourced.
  const restsOn: Grounding[] = moment.evidence.map((item) => ({
    key: `${item.type}-${item.id ?? item.label}`,
    quote: item.label,
    from: t(MOMENT_EVIDENCE_LABEL[item.type]),
  }));
  const footer = (
    <div className="pe-today-foot">
      <span>
        {plural("person.today.source", moment.evidence.length, {
          count: formatNumber(moment.evidence.length, locale),
        })}
      </span>
      {moment.freshness_at && (
        <>
          <span aria-hidden="true">·</span>
          <span>
            {t("person.today.updated", {
              when: freshness(moment.freshness_at, t, locale),
            })}
          </span>
        </>
      )}
    </div>
  );
  return (
    <>
      <CallCard
        name={name}
        standing={{
          label: t(MOMENT_RULE_LABEL[moment.rule]),
          tone: standingTone(moment.rule),
        }}
        // Why now, as the sentence the call rests on: the headline is the
        // ask, and it leads the day's work below.
        because={moment.why_now}
        restsOn={restsOn}
        footer={footer}
      >
        <RecordSpine
          source={spineSourceOf(view)}
          commercial={{ next_close_on: view.commercial?.deal?.close_date }}
        />
      </CallCard>
      <TodayPanel onOpenTasks={onOpenTasks}>
        {/* Rung 10 is a moment like any other: "nothing needs you today" is
            the answer a reader came for, and the verb the ladder still names
            on it — log what happened — rides the row like every other. */}
        <MomentMove key="moment" moment={moment} onAction={onAction} />
        {taskRows}
      </TodayPanel>
    </>
  );
}

// The move the ladder recommends, as the one row the agent is asking for. The
// headline is the ask; the verb at the row's end carries the server's own
// label for what performs it; and the evidence listed under the ask is the same
// list the call above rests on, because the move and the call were read from
// the same records.
function MomentMove({
  moment,
  onAction,
}: Readonly<{
  moment: PersonMoment;
  onAction: (action: PersonMomentAction) => void;
}>) {
  const secondary = moment.secondary_actions ?? [];
  return (
    <FoundMove
      title={moment.headline}
      basis={
        <ul className="pe-today-evidence">
          {moment.evidence.map((item) => (
            <li key={`${item.type}-${item.id ?? item.label}`}>
              {evidenceIcon(item.type)}
              <span>{item.label}</span>
            </li>
          ))}
        </ul>
      }
      action={
        <div className="pe-today-actions">
          <ActionVerb
            action={moment.recommended_action}
            primary
            onAction={onAction}
          />
          {/* Every other verb the moment carries, beside the one it leads with
              rather than in a second list elsewhere on the page, which would
              let the two disagree about what to do next. */}
          {secondary.map((action) => (
            <ActionVerb
              key={action.label}
              action={action}
              onAction={onAction}
            />
          ))}
        </div>
      }
    />
  );
}

// The open tasks already on this contact's record, quieter than the move
// above them: commitments the record already carries, which a reader scans.
//
// Rows rather than a component, because the panel counts what it is handed to
// decide whether the day is quiet — and a component that renders nothing is
// still one child. A withheld section yields no rows; the rail says what was
// withheld.
function useOpenTaskRows(view: Person360): ReactNode[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const tasks = (view.next_steps?.data ?? []).filter((task) => !task.is_done);
  // The assignee's name off the workspace roster, so the row's mark is the
  // colleague it sits with rather than an id. Asked for only when a task names
  // one.
  const roster = useRoster(
    "user",
    tasks.some((task) => Boolean(task.assignee_id)),
  );
  const nameOf = (userId: string | null | undefined): string | undefined => {
    if (!userId) {
      return undefined;
    }
    const entry = roster.data?.find((candidate) => candidate.id === userId);
    return entry && "display_name" in entry ? entry.display_name : undefined;
  };
  const asOf = Date.parse(view.as_of);
  return tasks
    .slice(0, OPEN_TASKS_SHOWN)
    .map((task) => (
      <TodoRow
        key={task.id}
        who={nameOf(task.assignee_id)}
        title={task.subject ?? t("person.moment.evidence.task")}
        due={taskDue(task, asOf, t, locale, zone)}
      />
    ));
}

// How many open tasks the day's work lists before the reader is sent to the
// full list. Three is what reads as a glance beside the move above them.
const OPEN_TASKS_SHOWN = 3;

// When a task is owed, coloured only once it is late. A task with no date says
// so rather than leaving the slot empty, which reads as a date that failed to
// load. Lateness is measured against the read's own `as_of`, so the row agrees
// with the dates beside it and does not drift while a tab is left open.
function taskDue(
  task: Activity,
  asOf: number,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): { label: string; tone?: "danger" } {
  if (!task.due_at) {
    return { label: t("co.next.undated") };
  }
  const { late } = daysPast(Date.parse(task.due_at), asOf);
  if (late) {
    return { label: t("co.next.overdue"), tone: "danger" };
  }
  return {
    label: t("co.next.due", { when: formatDate(task.due_at, locale, zone) }),
  };
}

/**
 * spineSourceOf reads the contact's 360 as the thread's source. The two
 * shapes agree on every field but the tasks: the spine wants a task's id under
 * `activity_id` and a settled `overdue`, and the 360 sends activity rows, so
 * the tasks are re-read here — from the same rows, against the same `as_of`.
 */
export function spineSourceOf(view: Person360): SpineSource {
  const asOf = Date.parse(view.as_of);
  return {
    as_of: view.as_of,
    last_inbound_at: view.last_inbound_at,
    last_outbound_at: view.last_outbound_at,
    activities: view.activities,
    next_steps: view.next_steps
      ? {
          data: view.next_steps.data
            .filter((task) => !task.is_done)
            .map((task) => ({
              activity_id: task.id,
              subject: task.subject ?? "",
              due_at: task.due_at,
              overdue: task.due_at
                ? daysPast(Date.parse(task.due_at), asOf).late
                : false,
            })),
        }
      : undefined,
  };
}

// One verb: the button, and under it what will happen when it is pressed.
function ActionVerb({
  action,
  primary,
  onAction,
}: Readonly<{
  action: PersonMomentAction;
  primary?: boolean;
  onAction: (action: PersonMomentAction) => void;
}>) {
  const t = useT();
  const blocked = action.state === "blocked";
  return (
    <span className="pe-today-verb">
      {/* Readiness is stated rather than left to a disabled button, because
          "you may not do this yet" and "this will ask you to confirm" are
          different answers. A blocked verb hands its sentence to the Button,
          whose `reason` bars the press AND describes the control with it — a
          `title` on a disabled button reaches no screen reader. The other
          states keep the caption under the verb. */}
      <Button
        variant={primary ? "primary" : "ghost"}
        small
        onClick={() => onAction(action)}
        reason={blocked ? readiness(action, t) : undefined}
      >
        {actionIcon(action.kind)}
        {action.label}
      </Button>
      {!blocked && (
        <span className="pe-today-verb-state">{readiness(action, t)}</span>
      )}
    </span>
  );
}

// A moment says what to do; it never says what carries it. So the reply verb is
// drawn as a send and not as an envelope — the envelope told a contact reached
// only over a chat channel that the button would mail them, which is neither
// what the action does nor something this card can know.
function actionIcon(kind: string): ReactNode {
  switch (kind) {
    case "schedule_meeting":
    case "open_meeting_brief":
    case "ask_colleague":
      return <Users size={15} aria-hidden="true" />;
    case "draft_reply":
      return <Send size={15} aria-hidden="true" />;
    case "complete_task":
      return <CheckSquare size={15} aria-hidden="true" />;
    case "open_research":
      return <Search size={15} aria-hidden="true" />;
    default:
      return <FileText size={15} aria-hidden="true" />;
  }
}

// What pressing it will do, in the three states the server distinguishes. A
// blocked action says so in words, not only by being unpressable: a disabled
// button carries no title a keyboard or touch reader ever sees, so the
// server's own reason (WHY this one is blocked) renders here when it sent
// one, and the generic word is the fallback for the rare blocked action that
// carries none.
function readiness(
  action: PersonMomentAction,
  t: ReturnType<typeof useT>,
): string {
  if (action.state === "will_confirm") {
    return t("person.rail.reviewFirst");
  }
  if (action.state === "blocked") {
    return action.blocked_reason ?? t("person.rail.blocked");
  }
  return t("person.rail.ready");
}

// A moment's evidence names a record TYPE and nothing about the transport, so
// there is no honest envelope to draw beside an activity: it is as likely to
// be a chat message or a call as a mail, and on a contact reached only over a
// channel the envelope was simply wrong. It is drawn as the record it is.
function evidenceIcon(type: string): ReactNode {
  switch (type) {
    case "task":
      return <CalendarDays size={15} aria-hidden="true" />;
    case "relationship_change":
      return <Users size={15} aria-hidden="true" />;
    default:
      return interactionIcon(null, 15);
  }
}

// The reader judges the age themselves, so this says when rather than how
// confident anything is. A deterministic rule shows no confidence meter.
function freshness(
  at: string,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const days = Math.floor((Date.now() - new Date(at).getTime()) / 86_400_000);
  if (days <= 0) {
    return t("person.today.freshToday");
  }
  if (days === 1) {
    return t("person.today.freshYesterday");
  }
  return t("person.today.freshDaysAgo", { count: formatNumber(days, locale) });
}

// The quiet-success state renders through the same cards: rung 10 is a moment
// like any other, and "nothing needs you today" is the answer a reader came
// for.
export function isQuiet(moment: PersonMoment): boolean {
  return moment.rule === "nothing_needed";
}
