import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, EmptyState } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import {
  calendarDaysBetween,
  formatDate,
  formatNumber,
} from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, useT } from "../i18n";
import { contactTabRoute } from "./contacttab";
import { useRoster } from "./entityref";
import { MoveButton } from "./movebutton";
import {
  basisAddsARecord,
  FoundMove,
  MOMENT_RULE_LABEL,
  MomentEvidence,
  TodayPanel,
  TodoRow,
  WithheldNotice,
} from "./record360";
import "./record360/record360.css";

// WHAT NEEDS A CONTACT TODAY, as the contact page assembles it: the move the
// server selected at the top of the panel, and the record's own open tasks
// under it.
//
// The panel is drawn on every contact, whatever rung the ladder reached. A
// quiet record and a thin relationship are ANSWERS to "what needs me" and the
// panel has a sentence for each; answering with no panel at all left the one
// question the page exists to answer with nothing standing where it belongs,
// and a reader who could not find it had no way to tell a quiet day from a
// pane that failed to load.

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];
type Activity = components["schemas"]["Activity"];

export function hasContactWork(view: Contact360): boolean {
  return (
    actionableMoment(view.moment) ||
    (view.next_steps?.data.some((task) => !task.is_done) ?? false)
  );
}

function actionableMoment(moment: ContactMoment | undefined): boolean {
  return Boolean(
    moment &&
      moment.rule !== "thin_relationship" &&
      moment.rule !== "nothing_needed",
  );
}

export function ContactToday({
  moment,
  view,
  onAction,
  onOpenTasks,
  onOpenEmail,
}: Readonly<{
  moment?: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenTasks?: () => void;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  // The one move the panel leads with, where the ladder found one. A quiet or
  // thin rung is an answer rather than an ask, so it draws no row and the
  // panel's own sentence speaks for the day.
  const move = moment && actionableMoment(moment) ? moment : undefined;
  const taskRows = useOpenTaskRows(view, move);
  const omitted = new Set(view.sections_omitted ?? []);
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      tasksLabel={t("today.workQueue")}
      notice={
        <>
          {/* How far the reading reached, under the answer rather than beside
              it: "nothing needs you" on a relationship with almost nothing
              recorded is only honest with the records it was read from named
              in the same breath. */}
          {moment?.rule === "thin_relationship" && (
            <p className="t-sub">{t("contact.overview.coverage")}</p>
          )}
          <WithheldNotice
            sections={[
              ...(omitted.has("moments") ? [t("today.source.moments")] : []),
              ...(omitted.has("next_steps")
                ? [t("today.source.nextSteps")]
                : []),
            ]}
          />
        </>
      }
    >
      {/* No moment read at all is not a quiet day: the panel keeps its place
          and says the reading is not shown, rather than drawing the quiet
          sentence, which is a claim about the record that a withheld or
          unread section gives it no basis for. */}
      {!moment && (
        <PanelBody>
          <EmptyState>{t("record.notShown")}</EmptyState>
        </PanelBody>
      )}
      {move && (
        <MomentMove
          moment={move}
          view={view}
          onAction={onAction}
          onOpenEmail={onOpenEmail}
        />
      )}
      {taskRows}
    </TodayPanel>
  );
}

// Evidence stays beside the action it supports.
function MomentMove({
  moment,
  view,
  onAction,
  onOpenEmail,
}: Readonly<{
  moment: ContactMoment;
  view: Contact360;
  onAction: (action: ContactMomentAction) => void;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const secondary = moment.secondary_actions ?? [];
  return (
    <FoundMove
      suggested
      // THIS contact's move first. The server writes the headline from the
      // record itself ("You owe them: call Alice back about the retrofit")
      // and the rule's own sentence is the same advice for everyone on that
      // rung, so the rule leads as the kicker and argues underneath while the
      // headline takes the row's loudest line. Read the other way round, a
      // reader met the template twice before the name of the thing to do.
      title={moment.headline}
      why={suggestionFor(moment, view, t, locale) ?? moment.why_now}
      kicker={t(MOMENT_RULE_LABEL[moment.rule])}
      basis={
        basisAddsARecord(moment) ? (
          <MomentEvidence
            evidence={moment.evidence}
            // The glyph names the KIND of record the move rests on, the same
            // way the brief's sources and the timeline's rows name theirs.
            kindOf={(item) =>
              item.type === "activity"
                ? view.activities?.data.find((row) => row.id === item.id)?.kind
                : item.type
            }
            onOpen={(item) => {
              const activity = view.activities?.data.find(
                (row) => row.id === item.id,
              );
              if (activity?.kind === "email" && onOpenEmail && item.id)
                onOpenEmail(item.id);
              else navigate(contactTabRoute(view.contact.id, "timeline"));
            }}
          />
        ) : undefined
      }
      action={
        // A fragment, not a column of its own: FoundMove owns the one
        // `.today-actions` column its row draws, defer included, so a second
        // column nested inside it laid the defer button beside these verbs
        // in a row instead of under them.
        <>
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
        </>
      }
    />
  );
}

// Why the rung fired, in the ladder's own words — the reason under the ask,
// which is the part a rep judges. Undefined where the rule has no sentence of
// its own: the server's `why_now` is then the only reason there is, and a
// reader is owed that rather than a blank line under the headline.
function suggestionFor(
  moment: ContactMoment,
  view: Contact360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string | undefined {
  switch (moment.rule) {
    case "gone_quiet": {
      // Counted from our last message: the silence is how long we have been
      // waiting. The day count is read off the record's own dates rather than
      // parsed back out of the server's sentence, and with no such date the
      // server's own sentence stands rather than a reason with a blank where
      // the count goes.
      const since = view.last_outbound_at;
      return since
        ? t("contact.moment.suggest.goneQuiet", {
            days: formatNumber(
              calendarDaysBetween(new Date(since), new Date(view.as_of)),
              locale,
            ),
          })
        : undefined;
    }
    case "re_engaged":
      return t("contact.moment.suggest.reEngaged");
    case "overdue_promise":
      return t("contact.moment.suggest.overduePromise");
    case "open_promise":
      return t("contact.moment.suggest.openPromise");
    case "job_change":
      return t("contact.moment.suggest.jobChange");
    case "public_signal":
      return t("contact.moment.suggest.publicSignal");
    case "missing_next_step":
      return t("contact.moment.suggest.missingNextStep");
    default:
      return undefined;
  }
}

// The open tasks already on this contact's record, quieter than the move
// above them: commitments the record already carries, which a reader scans.
//
// Rows rather than a component, because the panel counts what it is handed to
// decide whether the day is quiet — and a component that renders nothing is
// still one child. A withheld section yields no rows; the rail says what was
// withheld.
function useOpenTaskRows(
  view: Contact360,
  move: ContactMoment | undefined,
): ReactNode[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const named = recordsTheMoveNames(move);
  const tasks = (view.next_steps?.data ?? []).filter(
    (task) => !task.is_done && !named.has(task.id),
  );
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
        title={task.subject ?? t("task.untitled")}
        due={taskDue(task, asOf, t, locale, zone)}
        action={
          <MoveButton
            contactId={view.contact.id}
            move={{ action: "open_task", arguments: { activity_id: task.id } }}
          />
        }
      />
    ));
}

// The records the move above the list is itself about. A task the move names
// is the subject of that row, and listed again underneath it reads as a second
// thing to do — the same commitment counted twice, once as the ask and once as
// a chore.
function recordsTheMoveNames(move: ContactMoment | undefined): Set<string> {
  if (!move) {
    return new Set();
  }
  const verbs = [move.recommended_action, ...(move.secondary_actions ?? [])];
  return new Set([
    ...move.evidence.flatMap((item) => (item.id ? [item.id] : [])),
    // The verb's own destination too, for a move read out of the promise in a
    // conversation rather than out of the task somebody filed for it: the
    // evidence is then the message and only the button names the task.
    ...verbs.flatMap((verb) =>
      verb.destination?.surface === "task" && verb.destination.entity_id
        ? [verb.destination.entity_id]
        : [],
    ),
  ]);
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

// One verb: the button, and under it what will happen when it is pressed.
function ActionVerb({
  action,
  primary,
  onAction,
}: Readonly<{
  action: ContactMomentAction;
  primary?: boolean;
  onAction: (action: ContactMomentAction) => void;
}>) {
  const t = useT();
  const blocked = action.state === "blocked";
  // Every verb on this card is one the agent proposed and one the agent
  // carries out on the press: the brief it assembles, the research it runs,
  // the draft it writes into the composer. So the leading verb wears the AI
  // fill and the rest the plain outline, whatever the verb: one colour rule
  // for the column, read once. A green verb among them said "a human does
  // this one" about a draft the agent writes.
  const variant = primary ? "ai" : "ghost";
  return (
    <span className="today-verb">
      {/* A blocked verb hands its sentence to the Button, whose `reason`
          bars the press AND describes the control with it: a `title` on a
          disabled button reaches no screen reader. A verb that will ask for
          confirmation says nothing here; the confirmation IS the saying, and
          a caption announcing it under the button was the same step twice. */}
      <Button
        variant={variant}
        onClick={() => onAction(action)}
        reason={blocked ? blockedReason(action, t) : undefined}
      >
        {actionIcon(action.kind)}
        {action.label}
      </Button>
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

// Why this verb may not be pressed: the server's own reason when it sent one,
// and the generic word for the rare blocked action that carries none.
function blockedReason(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
): string {
  return action.blocked_reason ?? t("contact.rail.blocked");
}
