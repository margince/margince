import { CalendarDays, CheckSquare, FileText, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { interactionIcon } from "./interactionchrome";
import {
  BriefTitle,
  type Grounding,
  type StandingTone,
  VerdictHead,
} from "./record360";

// "Today with {first name}" (concept §5.5, ADR-0096 D2).
//
// ONE moment, chosen server-side by the fixed ladder. The client renders what
// it is given and computes nothing: a page that picked its own headline from
// date comparisons would drift from every other client showing the same
// record, which is the drift the rule/version stamp exists to make impossible.
//
// The action is a TYPED descriptor, so this renders only buttons whose
// destination the server named. An action with no destination still renders —
// some are their own destination — but one this client cannot route is not
// invented into a button that 404s.

type PersonMoment = components["schemas"]["PersonMoment"];
type PersonMomentAction = components["schemas"]["PersonMomentAction"];

// The rule that fired, in one word over the sentence it produced.
//
// The server picks exactly one rule from a fixed ladder, so the word is the
// server's judgement rather than this page's reading of a headline. Named
// rather than left implicit: "Gone quiet" and "Promise overdue" lead to
// different moves, and a reader who sees only the sentence has to infer which
// kind of thing they are looking at.
const MOMENT_RULE_LABEL = {
  meeting_prep: "person.moment.rule.meeting_prep",
  re_engaged: "person.moment.rule.re_engaged",
  job_change: "person.moment.rule.job_change",
  overdue_promise: "person.moment.rule.overdue_promise",
  overdue_task: "person.moment.rule.overdue_task",
  gone_quiet: "person.moment.rule.gone_quiet",
  open_promise: "person.moment.rule.open_promise",
  role_change: "person.moment.rule.role_change",
  public_signal: "person.moment.rule.public_signal",
  missing_next_step: "person.moment.rule.missing_next_step",
  thin_relationship: "person.moment.rule.thin_relationship",
  nothing_needed: "person.moment.rule.nothing_needed",
} as const satisfies Record<PersonMoment["rule"], MessageKey>;

const MOMENT_EVIDENCE_LABEL = {
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
function standingTone(rule: PersonMoment["rule"]): StandingTone {
  if (isLate(rule)) {
    return "warn";
  }
  return rule === "nothing_needed" ? "calm" : "accent";
}

// The three rules that mean somebody is being kept waiting. A promise past
// its date is late whether it was read out of an email or filed as a task, so
// both rungs colour the card the same; a promise not yet due is a live thread,
// not a warning.
function isLate(rule: PersonMoment["rule"]): boolean {
  return (
    rule === "gone_quiet" ||
    rule === "overdue_promise" ||
    rule === "overdue_task"
  );
}

export function PersonToday({
  moment,
  firstName,
  onAction,
}: Readonly<{
  moment: PersonMoment;
  firstName: string;
  onAction: (action: PersonMomentAction) => void;
}>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  // The amber treatment is the finding itself — a relationship that stopped,
  // or a promise that is late — so it colours the card rather than a badge
  // inside it.
  const warn = isLate(moment.rule);
  const secondary = moment.secondary_actions ?? [];
  // What the moment rests on, in the shape every claim on a record states it:
  // the label a reader can act on, and the kind of record it was read from.
  // Same disclosure as the account's call, because it is the same promise —
  // nothing a machine says here is unsourced.
  const restsOn: Grounding[] = moment.evidence.map((item) => ({
    key: `${item.type}-${item.id ?? item.label}`,
    quote: item.label,
    from: t(MOMENT_EVIDENCE_LABEL[item.type]),
  }));
  return (
    // The record's 360 card, in the person's own words: the same head every
    // written reading carries, tinted as the machine's work rather than as one
    // more thing to read.
    <Panel
      tone="ai"
      className="co-lead"
      title={<BriefTitle name={firstName} />}
      footer={
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
      }
    >
      {/* The call, then what it is standing on — the same head the account's
          own reading carries, so a rep who reads both records reads them the
          same way. */}
      <VerdictHead
        label={t(MOMENT_RULE_LABEL[moment.rule])}
        tone={standingTone(moment.rule)}
        because={moment.headline}
        restsOn={restsOn}
      />
      <PanelBody className="pe-today-lead">
        <div>
          <ul className="pe-today-evidence">
            {moment.evidence.map((item) => (
              <li key={`${item.type}-${item.id ?? item.label}`}>
                {evidenceIcon(item.type)}
                <span>{item.label}</span>
              </li>
            ))}
          </ul>

          {/* The rule that fired, named. A reader who disagrees with the verdict
              can see what produced it, which is the difference between a system
              that judges and one that explains. */}
          {warn && <p className="pe-today-rule">{moment.why_now}</p>}
        </div>

        {/* Every action the moment carries, with its readiness under it: the
            verbs live beside the moment they act on rather than in a second
            list elsewhere on the page, which would let the two disagree about
            what to do next. Readiness is stated rather than left to a disabled
            button, because "you may not do this yet" and "this will ask you to
            confirm" are different answers. */}
        <div className="pe-today-actions">
          <ActionVerb
            action={moment.recommended_action}
            primary
            onAction={onAction}
          />
          {secondary.map((action) => (
            <ActionVerb
              key={action.label}
              action={action}
              onAction={onAction}
            />
          ))}
        </div>
      </PanelBody>
    </Panel>
  );
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
  return (
    <span className="pe-today-verb">
      <Button
        variant={primary ? "primary" : "ghost"}
        onClick={() => onAction(action)}
        disabled={action.state === "blocked"}
        title={action.blocked_reason}
      >
        {actionIcon(action.kind)}
        {action.label}
      </Button>
      <span className="pe-today-verb-state">{readiness(action, t)}</span>
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

// The quiet-success state renders through the same component: rung 10 is a
// moment like any other, and "nothing needs you today" is the answer a reader
// came for rather than an empty card.
export function isQuiet(moment: PersonMoment): boolean {
  return moment.rule === "nothing_needed";
}
