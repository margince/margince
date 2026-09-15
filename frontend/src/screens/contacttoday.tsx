import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import {
  calendarDaysBetween,
  formatDate,
  formatNumber,
} from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { contactTabRoute } from "./contacttab";
import { useRoster } from "./entityref";
import { interactionIcon } from "./interactionchrome";
import { MoveButton } from "./movebutton";
import { FoundMove, TodayPanel, TodoRow, WithheldNotice } from "./record360";

// The server selects the recommendation. Empty relationship and quiet results
// describe coverage; they do not describe outstanding work.

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
  const taskRows = useOpenTaskRows(view);
  const omitted = new Set(view.sections_omitted ?? []);
  const withheld = (
    <WithheldNotice
      sections={[
        ...(omitted.has("moments") ? [t("today.source.moments")] : []),
        ...(omitted.has("next_steps") ? [t("today.source.nextSteps")] : []),
      ]}
    />
  );
  if (!actionableMoment(moment) && taskRows.length === 0) {
    // A quiet record already says so in the brief (its headline IS the
    // moment's) so the moment's second sentence under the brief card was the
    // same news twice, as a line floating between two panels. Only what the
    // brief cannot say stays: the thin-relationship coverage, and what was
    // withheld.
    if (moment?.rule === "nothing_needed") {
      return withheld;
    }
    return (
      <div className="contact-coverage t-sub">
        <p>
          {moment
            ? moment.rule === "thin_relationship"
              ? t("contact.moment.rule.thin_relationship")
              : moment.why_now
            : t("record.notShown")}
        </p>
        {moment?.rule === "thin_relationship" && (
          <p>{t("contact.overview.coverage")}</p>
        )}
        {withheld}
      </div>
    );
  }
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      tasksLabel={t("brief.feed.fullWorklist")}
      notice={withheld}
    >
      {moment && actionableMoment(moment) && (
        <MomentMove
          moment={moment}
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
  const suggestion = suggestionFor(moment, view, t, locale);
  return (
    <FoundMove
      suggested
      // The line under "Margince suggests" has to BE a suggestion. The
      // server's headline names the situation ("No reply for 174 days"), so
      // the card leads with the move that situation calls for and keeps the
      // situation as the reason under it.
      title={suggestion.ask}
      why={suggestion.why}
      basis={
        <ul className="pe-today-evidence">
          {[
            ...new Map(
              moment.evidence.map((item) => [
                `${item.type}:${item.id ?? item.label}`,
                item,
              ]),
            ).values(),
          ].map((item) => {
            // The glyph names the KIND of record the move rests on, the same
            // way the brief's sources and the timeline's rows name theirs.
            const activity = view.activities?.data.find(
              (row) => row.id === item.id,
            );
            const glyph = interactionIcon(
              item.type === "activity" ? activity?.kind : item.type,
            );
            return (
              <li
                key={`${item.type}-${item.id ?? item.label}`}
                className="t-sub"
              >
                {item.id ? (
                  <Button
                    variant="link"
                    onClick={() => {
                      if (activity?.kind === "email" && onOpenEmail && item.id)
                        onOpenEmail(item.id);
                      else
                        navigate(contactTabRoute(view.contact.id, "timeline"));
                    }}
                  >
                    {glyph}
                    {item.label}
                  </Button>
                ) : (
                  <span className="pe-source">
                    {glyph}
                    {item.label}
                  </span>
                )}
                {item.snippet && <q>{item.snippet}</q>}
              </li>
            );
          })}
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

// The ask and the reason for it, by rule. The server sends a headline that
// states the situation and a why_now that cites the rule; a reader under a
// "Margince suggests" label wants the move, so each rule leads with its verb
// and keeps the situation as the reason. A rule whose headline already IS a
// move (meeting prep) keeps it. The day count is read off the record's own
// dates rather than parsed back out of the server's sentence.
function suggestionFor(
  moment: ContactMoment,
  view: Contact360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): { ask: string; why: string } {
  const situation = { ask: moment.headline, why: moment.why_now };
  const lead = (key: MessageKey, params?: Record<string, string>) => ({
    ask: t(key, params),
    why: moment.headline,
  });
  switch (moment.rule) {
    case "gone_quiet": {
      // Counted from our last message: the silence is how long we have been
      // waiting. With no such date the server's own sentence stands, rather
      // than a suggestion with a blank where the count goes.
      const since = view.last_outbound_at;
      if (!since) {
        return situation;
      }
      return lead("contact.moment.suggest.goneQuiet", {
        days: formatNumber(
          calendarDaysBetween(new Date(since), new Date(view.as_of)),
          locale,
        ),
      });
    }
    case "re_engaged":
      return lead("contact.moment.suggest.reEngaged");
    case "overdue_promise":
      return lead("contact.moment.suggest.overduePromise");
    case "open_promise":
      return lead("contact.moment.suggest.openPromise");
    case "job_change":
      return lead("contact.moment.suggest.jobChange");
    case "public_signal":
      return lead("contact.moment.suggest.publicSignal");
    case "missing_next_step":
      return lead("contact.moment.suggest.missingNextStep");
    default:
      return situation;
  }
}

// The open tasks already on this contact's record, quieter than the move
// above them: commitments the record already carries, which a reader scans.
//
// Rows rather than a component, because the panel counts what it is handed to
// decide whether the day is quiet — and a component that renders nothing is
// still one child. A withheld section yields no rows; the rail says what was
// withheld.
function useOpenTaskRows(view: Contact360): ReactNode[] {
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
    <span className="pe-today-verb">
      {/* A blocked verb hands its sentence to the Button, whose `reason`
          bars the press AND describes the control with it: a `title` on a
          disabled button reaches no screen reader. A verb that will ask for
          confirmation says nothing here; the confirmation IS the saying, and
          a caption announcing it under the button was the same step twice. */}
      <Button
        variant={variant}
        small
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
