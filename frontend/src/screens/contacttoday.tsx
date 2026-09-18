import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button, EmptyState } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { formatDate, formatNumber } from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, useT } from "../i18n";
import { contactTabRoute } from "./contacttab";
import { useRoster } from "./entityref";
import { MoveButton } from "./movebutton";
import {
  basisAddsARecord,
  FoundMove,
  MomentEvidence,
  momentKicker,
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
// quiet record and a thin relationship are ANSWERS to "what needs me", and
// each has a sentence of its own inside the panel: the question the page
// exists to answer is answered where it is asked, and a reader can always
// tell a quiet day from a pane that has not loaded.

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
  const { locale } = useLocale();
  const move = moment && actionableMoment(moment) ? moment : undefined;
  const tasks = useOpenTaskRows(view, move);
  const standing = standingSentence(moment, t);
  const omitted = new Set(view.sections_omitted ?? []);
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      tasksLabel={t("today.workQueue")}
      footer={panelFoot({ moment, more: tasks.more, t, locale })}
      notice={
        <WithheldNotice
          sections={[
            ...(omitted.has("moments") ? [t("today.source.moments")] : []),
            ...(omitted.has("next_steps") ? [t("today.source.nextSteps")] : []),
          ]}
        />
      }
    >
      {/* Neither a move nor a quiet day. The panel's own sentence claims that
          nothing needs the reader, and an unread reading and a relationship
          with nothing recorded are both short of the basis for that claim, so
          each says what it does know in the quiet line's place. */}
      {standing && (
        <PanelBody>
          <EmptyState>{standing}</EmptyState>
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
      {tasks.rows}
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
  const secondary = moment.secondary_actions ?? [];
  return (
    <FoundMove
      suggested
      // THIS contact's move first: the server writes the headline from the
      // record itself ("You owe them: call Alice back about the retrofit"),
      // and it takes the row's loudest line while the rule it fired on
      // qualifies the byline beside it.
      title={moment.headline}
      // The server's `why_now` is the ONLY reason, here and on the account
      // brief: one moment gives one reason, and the facts in it ("no reply
      // after 14 days is the rule; yours went out 131 days ago") are the part
      // a rep judges. A sentence composed from the rule instead would be the
      // same words for every contact on that rung.
      why={moment.why_now}
      kicker={momentKicker(moment, t)}
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

// What the body says where there is no move to draw. The panel's own quiet
// line is a claim that nothing needs the reader; a reading nobody may see and
// a relationship with nothing recorded are both short of the basis for it, so
// each stands in its place — the first says the reading is not shown, the
// second says what the server actually found. The quiet rung has the basis,
// and leaves the line to the panel.
function standingSentence(
  moment: ContactMoment | undefined,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!moment) {
    return t("record.notShown");
  }
  return moment.rule === "thin_relationship" ? moment.headline : undefined;
}

// The band under the rows: what the list did not show, and how far the reading
// reached. Both are facts about the READING rather than work, so they sit in
// the foot; as rows they would read as two more things to do. A truncated list
// with no count reads as "that is everything", which is the one thing a list
// of commitments may not say.
function panelFoot({
  moment,
  more,
  t,
  locale,
}: Readonly<{
  moment: ContactMoment | undefined;
  more: number;
  t: ReturnType<typeof useT>;
  locale: Locale;
}>): ReactNode | undefined {
  const lines = [
    ...(more > 0
      ? [t("co.suggest.more", { count: formatNumber(more, locale) })]
      : []),
    // "Nothing needs you" about a relationship with almost nothing recorded is
    // honest only with the reach of the records behind it named beside it.
    ...(moment?.rule === "thin_relationship"
      ? [t("contact.overview.coverage")]
      : []),
  ];
  if (lines.length === 0) {
    return undefined;
  }
  return (
    <>
      {lines.map((line) => (
        <p key={line} className="t-caption">
          {line}
        </p>
      ))}
    </>
  );
}

// The open tasks already on this contact's record, quieter than the move
// above them: commitments the record already carries, which a reader scans.
//
// Rows rather than a component, because the panel counts what it is handed to
// decide whether the day is quiet — and a component that renders nothing is
// still one child. A withheld section yields no rows; the rail says what was
// withheld. `more` is what the cut left out, which the foot reports.
function useOpenTaskRows(
  view: Contact360,
  move: ContactMoment | undefined,
): { rows: ReactNode[]; more: number } {
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
  return {
    rows: tasks.slice(0, OPEN_TASKS_SHOWN).map((task) => (
      <TodoRow
        key={task.id}
        who={nameOf(task.assignee_id)}
        title={task.subject ?? t("task.untitled")}
        due={taskDue(task, asOf, t, locale, zone)}
        action={
          <MoveButton
            contactId={view.contact.id}
            move={{
              action: "open_task",
              arguments: { activity_id: task.id },
            }}
          />
        }
      />
    )),
    more: Math.max(tasks.length - OPEN_TASKS_SHOWN, 0),
  };
}

// The records the move above the list is itself about. A task the move names
// is the subject of that row, and listed again underneath it reads as a second
// thing to do — the same commitment counted twice, once as the ask and once as
// a chore.
// Read off the evidence alone, which is where the server names a record: a
// destination is a surface to open (composer, brief, research, record, the log
// form) and never a second naming of the row the move is about.
function recordsTheMoveNames(move: ContactMoment | undefined): Set<string> {
  return new Set(
    (move?.evidence ?? []).flatMap((item) => (item.id ? [item.id] : [])),
  );
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
  // Indigo marks the verb the agent RECOMMENDS, not work the agent performs:
  // every verb in this column was proposed by the moment, and the leading one
  // is the move it is asking for. So the lead wears the AI fill and the rest
  // the plain outline, whatever each verb goes on to do — one colour rule for
  // the column, read once.
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
