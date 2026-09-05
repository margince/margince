import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { calendarDay } from "../format/calendarday";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import type { Locale, useT } from "../i18n";
import { translatePlural } from "../i18n";
import { BRIEF_PARAM, COMPOSE_PARAM, THREAD_PARAM } from "./personpage.address";
import { settingsAddress } from "./settingsnav";
import type {
  Worklist,
  WorklistComparison,
  WorklistFilter,
  WorklistItem,
  WorklistReason,
  WorklistValue,
} from "./worklist.queries";

type T = ReturnType<typeof useT>;

// The queue's words.
//
// The server sends facts and closed vocabularies; every sentence on the page is
// written here, in the reader's own language. That split is why the ranking can
// be explained at all: a reason composed server-side would reach a German
// reader in English, so what travels is `{kind, value}` and the phrase is the
// client's to write.

// Which record an item points at, as an address the router understands.
//
// Through the entity registry, never a switch written here: the record types
// have route names of their own (`contacts`, not `people`), and a second
// spelling of them sends a reader to a page that does not exist. An activity
// resolves to nothing on purpose — it is a timeline entry rather than a record
// with a page, so naming it on the row is honest and linking it is not.
export function subjectHref(item: WorklistItem): string | undefined {
  const subject = item.subject;
  if (!subject || !isEntityKind(subject.type)) {
    return undefined;
  }
  return routeHash(ENTITY[subject.type].route(subject.id));
}

// Where a row goes when it names no record of its own.
//
// Most rows point at a record and reach it through the entity registry. A few
// name a QUEUE instead — a data-subject request is worked on the privacy
// screen and nowhere else — and without a destination those rows are a
// sentence a reader cannot follow, on the one lane whose whole argument is a
// legal clock somebody has to answer.
//
// A destination is not a verb: these rows still offer no action, because the
// queue cannot perform one. It is the difference between telling somebody
// where a room is and claiming to have opened the door.
// Through settingsAddress, not a path spelled here: the settings registry
// decides whether a tab sits under the admin segment, and a second spelling of
// that decision would keep pointing at the old address the day it moves.
const SOURCE_QUEUE: Partial<Record<WorklistItem["source"], string>> = {
  dsr: routeHash(settingsAddress("privacy")),
  // A rule that failed, and the page that lists the rules.
  //
  // The row named a broken automation and offered no address at all — not the
  // run, not the rule, not the screen either lives on — so a reader was told
  // something was wrong and left to go and find it. The queue still performs
  // nothing here: fixing a rule is the automations page's job, and this is the
  // difference between saying where a room is and claiming to have opened the
  // door.
  //
  // NOT a retry. A re-run would have to replay the event that fired the rule,
  // and `workflow_run` does not keep it — the row holds a pointer to a bus
  // event the bus drops after about three days, so a retry would silently do
  // nothing on day four. Offering one would be the promise this table exists
  // to avoid making.
  //
  // The `ai` tab is where the automations list lives, and its read is the one
  // every seeded role holds — so this address answers for a rep as well as for
  // an operator rather than routing most readers into a refusal.
  automation_run: routeHash(settingsAddress("ai")),
  // The same page answers for the AI work that a rule set off, for the same
  // reason and with the same limit.
  ai_work_health: routeHash(settingsAddress("ai")),
};

// The address a row's headline links to: its record where it has one, the
// queue that owns it where it does not, and nothing at all for a row that is
// neither — a system condition fixed on a settings screen the card does not
// pretend to know.
export function rowHref(item: WorklistItem): string | undefined {
  return askHref(item) ?? subjectHref(item) ?? SOURCE_QUEUE[item.source];
}

// An introduction ask goes to the contact's NETWORK tab, not the contact.
//
// The ask is answered there and nowhere else, and the default tab is a page
// that does not mention it. Landing a colleague on the contact's overview and
// leaving them to find the right tab is the hand-off this queue exists to
// remove — they came here because somebody is waiting on them.
export function askHref(item: WorklistItem): string | undefined {
  if (
    item.source !== "introduction_request" ||
    item.subject?.type !== "person"
  ) {
    return undefined;
  }
  return routeHash({ screen: "contacts", id: item.subject.id, id2: "network" });
}

// One comparator value, in the reader's notation.
//
// A magnitude written in the wrong notation is a different number to the person
// reading it: a German reader reads "1.234", and the coerced form says
// something else.
function valueText(
  value: WorklistValue | undefined,
  locale: Locale,
  zone: string,
): string | null {
  if (!value) {
    return null;
  }
  switch (value.kind) {
    case "date":
      return value.date ? formatDateTime(value.date, locale, zone) : null;
    case "money":
      // Both halves or nothing. A euro sign on a figure that might be dong is
      // worse than no figure, and the scale itself is wrong without the
      // currency — so a value that names none says nothing at all.
      return value.minor == null || !value.currency
        ? null
        : formatMoney(value.minor, value.currency, locale);
    case "days":
      return value.days == null ? null : formatNumber(value.days, locale);
    case "level":
      return value.level == null ? null : formatNumber(value.level, locale);
    default:
      return null;
  }
}

// The reasons that read differently with a figure in them and whose figure is
// a currency amount, not a count — a money figure never needs the reader's
// plural rule, which is what sets these apart from DAYS_VALUED_REASONS below.
// Spelled as a set rather than inferred from whether a value arrived: a value
// can travel for a reason whose sentence has nowhere to put it, and a key
// composed from that would not exist.
const VALUED_REASONS = {
  expected_revenue: true,
  material: true,
  below_material: true,
  // The lead's own deadline, which is a MOMENT rather than a figure: valueText
  // renders a date value in the reader's locale and zone, so the sentence says
  // when without this file composing one.
  response_due_soon: true,
} as const;

type ValuedReason = keyof typeof VALUED_REASONS;

function valued(kind: WorklistReason["kind"]): kind is ValuedReason {
  return kind in VALUED_REASONS;
}

// The valued reasons whose figure is a DAY COUNT rather than a currency
// amount. "1 days" is a different kind of wrong from "1 day" — a count needs
// the reader's own plural rule, which a money figure never does.
const DAYS_VALUED_REASONS = {
  waiting_days: true,
  quiet_days: true,
} as const;

function daysValued(
  kind: WorklistReason["kind"],
): kind is keyof typeof DAYS_VALUED_REASONS {
  return kind in DAYS_VALUED_REASONS;
}

// Every reason this client has a sentence for.
//
// A newer server sending a reason this build does not know must not print
// `worklist.because.customer_escalated` at a reader — a missing translation
// returns its own key, so an unrecognised value has to be caught here rather
// than discovered on screen.
const KNOWN_REASONS = {
  pinned: true,
  buyer_wrote_last: true,
  waiting_days: true,
  overdue: true,
  due_today: true,
  closing_soon: true,
  expected_revenue: true,
  material: true,
  below_material: true,
  quiet_days: true,
  no_champion: true,
  promised: true,
  approved_and_failed: true,
  blocks_customer_work: true,
  routine: true,
  repeated_failure: true,
  legal_deadline: true,
  meeting_soon: true,
  meeting_unprepared: true,
  response_overdue: true,
  response_due_soon: true,
  unassigned: true,
  stale: true,
  no_reply_history: true,
  asks_nothing: true,
} as const;

type KnownReason = keyof typeof KNOWN_REASONS;

function known(kind: WorklistReason["kind"]): kind is KnownReason {
  return kind in KNOWN_REASONS;
}

// The comparators this build can name, for the same reason.
//
// `crowded` reads the other way round from the rest: this row is above because
// the one BELOW was held back, so a single lane could not own the page. The
// server sends no values with it deliberately — "8th against 9th" is a fact
// about the lane rather than about either row. Absent from this list it drew
// nothing at all, on exactly the row whose position most needs explaining.
const KNOWN_COMPARATORS = {
  pin: true,
  level: true,
  deadline: true,
  expected_revenue: true,
  waiting_days: true,
  relationship: true,
  crowded: true,
} as const;

function knownComparator(
  comparator: WorklistComparison["comparator"],
): comparator is keyof typeof KNOWN_COMPARATORS {
  return comparator in KNOWN_COMPARATORS;
}

// One fact behind an item's rank.
/**
 * The reason kind that reads as a BADGE rather than as a phrase in the line.
 *
 * "Nothing prepared" is a state of the meeting, the way `overdue` is a state of
 * a deadline — not one weighed fact among several. Buried mid-sentence in a
 * `because` line it reads as background; a rep scanning a rail of meetings for
 * the one to open before it starts needs it at a glance.
 *
 * It is therefore drawn once, as a badge, and left OUT of the phrase line. Said
 * in both places it would read as two separate findings about one meeting.
 */
const BADGED = "meeting_unprepared";

/** Whether this row's meeting has nothing prepared for it. */
export function isUnprepared(item: WorklistItem): boolean {
  return item.because.some((reason) => reason.kind === BADGED);
}

/**
 * The reason kinds the WHEN line already says.
 *
 * Each is the moment in a coarser register: a task row printed "due 06.07.2026,
 * 15:00" and then "due today" underneath it, an overdue one said "overdue" a
 * third time beside the badge that already says so, and a meeting said "starts
 * 14:12" over "starting shortly". One clock, twice, reading as two findings.
 * The moment wins because it names the hour a rep is racing rather than the day.
 *
 * A SET rather than one kind, because the duplication is structural. `overdue`
 * and `due_today` are the two arms of a single `if` in the task lane, both
 * guarded by the same `due_at` the moment is drawn from, so a rule naming one
 * of them names half a condition. `meeting_soon` fires only where the meeting
 * has a `due_at`, which is exactly when its own when line is drawn, so it has
 * no non-duplicating case at all.
 *
 * The other deadline reasons stay OUT of this set and that is the non-obvious
 * half: `closing_soon`, `response_overdue` and `response_due_soon` ride on
 * sources — `deal_at_risk`, `brief_item`, `lead_response` — for which
 * `whenKeyFor` answers null. Nothing draws their moment, so their phrase is the
 * only place the fact is said.
 *
 * DROPPED ONLY WHERE THE MOMENT IS DRAWN. A row whose `due_at` the when line
 * refuses — an approval's lapse instant, which is a fact about the staged work
 * and not a deadline the rep owes — still says its phrase, or the row would
 * lose the fact entirely rather than say it once.
 */
const SAID_BY_THE_WHEN_LINE = new Set<string>([
  "due_today",
  "overdue",
  "meeting_soon",
]);

/** The reasons a row says in its phrase line — everything said elsewhere. */
export function phrasedReasons(
  item: WorklistItem,
  whenDrawn: boolean,
): WorklistReason[] {
  return item.because.filter(
    (reason) =>
      reason.kind !== BADGED &&
      !(whenDrawn && SAID_BY_THE_WHEN_LINE.has(reason.kind)),
  );
}

export function reasonText(
  reason: WorklistReason,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (!known(reason.kind)) {
    // A reason this build has no sentence for is DROPPED, not printed as its
    // own key. The row keeps its other reasons and says one thing less.
    return null;
  }
  const value = valueText(reason.value, locale, zone);
  if (
    value !== null &&
    daysValued(reason.kind) &&
    reason.value?.kind === "days" &&
    reason.value.days != null
  ) {
    const base = `worklist.because.${reason.kind}.value` as const;
    return translatePlural(locale, base, reason.value.days, { value });
  }
  if (value !== null && valued(reason.kind)) {
    return t(`worklist.because.${reason.kind}.value` as const, { value });
  }
  return t(`worklist.because.${reason.kind}` as const);
}

// The comparators whose sentence names BOTH sides. A level or a pin decided
// without a figure a reader would compare, so those say what decided and stop.
const PAIRED_COMPARATORS = {
  deadline: true,
  expected_revenue: true,
  waiting_days: true,
} as const;

type PairedComparator = keyof typeof PAIRED_COMPARATORS;

function paired(
  comparator: WorklistComparison["comparator"],
): comparator is PairedComparator {
  return comparator in PAIRED_COMPARATORS;
}

// Why this row sits above the next one.
//
// The comparator that DECIDED, with both sides' values — so a reader can check
// the order rather than trust it. `order` means every comparator tied and the
// ids broke it, which is not a reason a person needs to read, so it draws
// nothing at all.
export function comparisonText(
  comparison: WorklistComparison | undefined,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (
    !comparison ||
    comparison.comparator === "order" ||
    !knownComparator(comparison.comparator)
  ) {
    return null;
  }
  const mine = valueText(comparison.mine, locale, zone);
  const theirs = valueText(comparison.theirs, locale, zone);
  if (mine === null || theirs === null || !paired(comparison.comparator)) {
    return t(`worklist.above.${comparison.comparator}` as const);
  }
  return t(`worklist.above.${comparison.comparator}.pair` as const, {
    mine,
    theirs,
  });
}

// What happens if the reader does nothing.
export function consequenceText(item: WorklistItem, t: T): string | null {
  if (item.consequence === "none") {
    return null;
  }
  return t(`worklist.consequence.${item.consequence}` as const);
}

// The deal's own figures, in one line: what it is worth, and when it closes.
//
// The concept's sharpest example was a €160,100 deal reduced to "no contact for
// 83 days". The money was on the wire the whole time; only the row discarded it.
export function dealFactsText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  const deal = item.deal;
  if (!deal) {
    return null;
  }
  const parts: string[] = [];
  if (deal.amount_minor != null && deal.currency) {
    parts.push(formatMoney(deal.amount_minor, deal.currency, locale));
  }
  if (deal.expected_close_date) {
    parts.push(
      t("worklist.deal.closes", {
        date: formatDate(deal.expected_close_date, locale, zone),
      }),
    );
  }
  return parts.length > 0 ? parts.join(" · ") : null;
}

// The MOMENT a dated row is racing: when a meeting starts, when a task is due.
//
// `due_at` reached the client on both and nothing read it. A meeting said
// "starting shortly" — the same three words whether it began in four minutes or
// in fifty — so the one row a rep has to open BEFORE a wall-clock time was the
// row that would not say the time. A task said "Overdue" and left the reader to
// go and find out by how long.
//
// The two are one function because they are one question to the reader: what
// clock am I against. What differs is only which side of now the answer is on.
//
// Today's meeting shows the CLOCK TIME and nothing else — a rep reads this at
// their desk on the morning it matters, and "today" is the frame they are
// already in. Anything else shows the date, because a bare "09:00" on a row two
// days out is a time the reader will act on this morning.
export function whenText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  zone: string,
  now: Date,
): string | null {
  if (!item.due_at) {
    return null;
  }
  const key = whenKeyFor(item);
  if (key === null) {
    return null;
  }
  return t(key, { when: momentText(item.due_at, locale, zone, now) });
}

// Which sentence the moment goes in — and null for a row whose `due_at` is not
// a clock the reader is racing.
//
// An approval's `due_at` is when the proposal LAPSES, which is a fact about the
// staged work rather than a deadline the rep owes; the contract says so where
// the field is declared. Drawing it as "due" would turn "this offer goes stale"
// into "you are late", which is the row telling the reader something untrue
// about their own day.
function whenKeyFor(
  item: WorklistItem,
): "worklist.when.starts" | "worklist.when.due" | null {
  if (item.source === "meeting") {
    return "worklist.when.starts";
  }
  if (item.source === "task") {
    return "worklist.when.due";
  }
  return null;
}

// The moment itself: the time alone when it falls today, the date and time
// otherwise.
function momentText(
  dueAt: string,
  locale: Locale,
  zone: string,
  now: Date,
): string {
  return sameDayInZone(dueAt, now, zone)
    ? formatTimeOfDay(dueAt, locale, zone)
    : formatDateTime(dueAt, locale, zone);
}

// Whether an instant falls on the reader's own calendar day.
//
// Through `calendarDay`, which already answers "which day is this, there" — a
// second formatter spelled here would be a second answer to that question, and
// the two would drift the first time either changed.
//
// Compared in the VIEWER's zone rather than the runner's, because the whole row
// is drawn in that zone: a meeting at 23:30 in Berlin, read on a machine set to
// UTC, is still tonight's meeting to the person reading it.
function sameDayInZone(utcIso: string, now: Date, zone: string): boolean {
  return calendarDay(new Date(utcIso), zone) === calendarDay(now, zone);
}

// Where the row's suggested step leads.
//
// The RECORD the thread belongs to, not the message: an activity is a timeline
// entry with no page of its own — `app/entity.ts` says so in as many words —
// so `#/activities/<id>` would be a control that goes nowhere.
//
// A `draft_reply` row whose subject is a PERSON opens the composer, through
// `?compose=reply` on their record (personpage.tsx COMPOSE_PARAM). Everything
// else lands on the record itself, and its verb says so.
//
// WHY ONLY A PERSON. The composer lives on the person page and drafts to the
// person, choosing its transport from their own reachability. A deal or an
// organization has no composer to open, so a link claiming to draft there would
// promise what the click cannot do — the defect this function's own comment
// warned about before the route existed.
//
// AND IT NAMES THE MESSAGE. `?thread=<activity>` carries the conversation this
// row is about, so the composer opens on the transport that message is on
// rather than on whichever this contact leads with. Without it a contact
// reachable two ways had the reply drafted into the wrong one — which is why
// this function returned a bare record href until the composer could be aimed.
//
// `draft_email` carries none, and must not: it OPENS a conversation rather than
// continuing one, so a thread id would anchor it on something the reader is not
// answering.
// The verbs this row can take a reader to.
//
// Both end at a person's composer, and which of the two the server chose
// reaches the address as well as the label: `draft_reply` names the message it
// answers, `draft_email` names none because it is opening a conversation.
//
// `open_meeting_brief` is the third, and it lands somewhere else entirely: the
// brief is read as `?prep=<activity>` on the PERSON's record, so the address
// needs an id the subject does not carry. The server sends it as `with_person`,
// and only where the meeting names somebody this reader may see.
//
// It used to be described here as PERFORMED rather than navigated — "opens a
// drawer, and neither is a thing a link can do" — which was true of the drawer
// and false of the way in. The row could describe a meeting and offer no way to
// prepare for it, which is the one thing a rep opens that row to do.
//
// The remaining verbs are absent, each for its own reason. `create_task` posts
// a task body, which is a write rather than a destination. `none` names no
// step. `reconnect` leaves for a provider's consent screen, which is a handoff.
// A row keeps its own verbs in every case, so none of these leaves the reader
// with nothing.
const NAVIGABLE_MOVES = new Set([
  "draft_reply",
  "draft_email",
  "open_meeting_brief",
  "open_task",
]);

/**
 * Whether a move has what its own verb needs.
 *
 * `draft_reply` answers a MESSAGE, and the contract says a client draws a
 * control only where the operand its verb needs is present. One carrying no
 * `activity_id` names nothing to answer — schema-valid, since the field is
 * optional for the verbs that take no record, and undrawable all the same.
 *
 * `draft_email` needs none: an opening outreach is a first message to a person,
 * and there is no earlier record for it to name.
 */
function moveIsComplete(move: NonNullable<WorklistItem["move"]>): boolean {
  return (
    !["draft_reply", "open_task"].includes(move.action) ||
    move.activity_id !== undefined
  );
}

/**
 * The brief's address: which meeting, on whose page.
 *
 * BOTH ids, because neither names it. The activity says which meeting to brief
 * and the person says whose record it opens on — the brief is not a page of its
 * own. A row missing either names nothing openable and draws no control, which
 * is the same promise every other verb here makes about its own operand.
 */
function briefHref(item: WorklistItem): string | undefined {
  const meeting = item.move?.activity_id;
  if (!meeting || !item.with_person) {
    return undefined;
  }
  const person = routeHash(ENTITY.person.route(item.with_person));
  return `${person}?${BRIEF_PARAM}=${meeting}`;
}

export function moveHref(item: WorklistItem): string | undefined {
  const move = item.move;
  if (!move || !NAVIGABLE_MOVES.has(move.action) || !moveIsComplete(move)) {
    return undefined;
  }
  if (move.action === "open_meeting_brief") {
    return briefHref(item);
  }
  const record = subjectHref(item);
  if (move.action === "open_task") return record;
  if (!record || item.subject?.type !== "person") {
    return record;
  }
  // The message the row is about travels with the ask. Without it the composer
  // would open on whichever transport this contact leads with, which for a
  // contact reachable two ways is a reply drafted into the wrong conversation —
  // the overstated promise this link was withheld for until it could be kept.
  const thread = move.activity_id;
  const anchored = thread
    ? `&${THREAD_PARAM}=${encodeURIComponent(thread)}`
    : "";
  return `${record}?${COMPOSE_PARAM}=reply${anchored}`;
}

/**
 * Whether this row's move opens the composer, rather than only a record.
 *
 * Asked of the SAME completeness rule moveHref uses, so the label and the link
 * cannot disagree: a move this refuses to draw must not also be described as
 * one that drafts.
 */
export function moveOpensComposer(item: WorklistItem): boolean {
  const move = item.move;
  return (
    move !== undefined &&
    NAVIGABLE_MOVES.has(move.action) &&
    moveIsComplete(move) &&
    item.subject?.type === "person"
  );
}

// What the move's control says.
//
// THE LABEL FOLLOWS THE VERB THE SERVER CHOSE. One hardcoded label was right
// while `draft_reply` was the only verb; with several it would promise a reply
// over a link that opens a fresh message. It follows the ROUTE too: where the
// composer opens, the verb is the act; where the link only reaches the record,
// it says so.
export function moveLabel(item: WorklistItem, t: T): string {
  if (item.move?.action === "open_task") return t("deal360.openTask");
  const opens = moveOpensComposer(item);
  if (item.move?.action === "draft_email") {
    return t(
      opens ? "worklist.verb.draft_email_now" : "worklist.verb.draft_email",
    );
  }
  return t(
    opens ? "worklist.verb.draft_reply_now" : "worklist.verb.draft_reply",
  );
}

// One item's headline.
//
// The server sends a sentence where it HAS one — an approval summary composed
// at staging time, a deal's own name, a message's subject. Where it has none
// the sentence would have to be invented, and the client writes it from the
// source instead.
export function itemTitle(item: WorklistItem, t: T, locale: Locale): string {
  // A group names itself by what it holds and how much: "43 likely automated
  // senders" is the whole row, and a reader decides whether to open it from
  // that sentence alone.
  if (item.batch) {
    // "200+" where the read stopped at its own bound. A floor printed as a
    // total is a wrong number rather than a bounded one, and the reader has no
    // way to tell the two apart.
    const count = item.batch.at_least
      ? `${formatNumber(item.batch.count, locale)}+`
      : formatNumber(item.batch.count, locale);
    // An incident names WHAT is broken; a hygiene group names its kind.
    //
    // From `label`, never from `cause`. The cause is the identity the group was
    // formed on and reads like one — interpolating it printed
    // "automation_run:01a0…-… failed 12 times" at a rep, which names nothing
    // they can act on and cannot be told from a bug. A group whose lane minted
    // no name falls back to the generic phrase rather than to the identity.
    if (item.batch.key === "system_incident") {
      return t("worklist.batch.system_incident", {
        count,
        cause: item.batch.label ?? t("worklist.batch.unnamedCause"),
      });
    }
    return t(`worklist.batch.${item.batch.key}` as const, { count });
  }
  if (item.title) {
    // A title that names no record, on a row that HAS one, gets the record's
    // name beside it. Eight rows reading "Follow up with the new lead" cannot
    // be told apart or put in order, and the lead's name is already on the row
    // — it was only the sentence that discarded it.
    const named = item.subject?.label;
    return named && !item.title.includes(named)
      ? `${item.title} · ${named}`
      : item.title;
  }
  if (!knownSource(item.source)) {
    return t("worklist.untitled.generic");
  }
  return t(`worklist.untitled.${item.source}` as const);
}

// Which source could not be read, and why, in words.
//
// The source name goes through the same known-source check the titles use, so
// a source this build has never heard of is described generically rather than
// printed as its own identifier.
//
// What `sourceName` returns is a row TITLE, and most of them are whole clauses
// — so the frame names the fact first and lets the title follow a colon. Read
// as a subject instead, fourteen of the twenty-one ran two sentences together.
// The alternative is a second vocabulary spelling each source as a noun, in
// every language, kept beside the titles; the colon costs nothing and cannot
// drift.
export function sourceUnavailableText(
  missing: NonNullable<Worklist["sources_unavailable"]>[number],
  t: T,
): string {
  const name = sourceName(missing.source, t);
  return missing.reason === "withheld"
    ? t("worklist.source.withheld", { source: name })
    : t("worklist.source.failed", { source: name });
}

// One source, in the reader's words.
//
// Through the same known-source check the titles use, so a source this build
// has never heard of is described generically rather than printed as its own
// identifier — a reader must never be shown `ai_work_health` as a noun.
export function sourceName(source: string, t: T): string {
  return knownSource(source as WorklistItem["source"])
    ? t(`worklist.untitled.${source as keyof typeof KNOWN_SOURCES}`)
    : t("worklist.untitled.generic");
}

// The sources this build can name without its own sentence.
//
// Exported so a gate over these sentences derives its corpus from here rather
// than keeping a second copy of the list: a hand-maintained census goes short
// of its subject the moment a source is added, and going short is the one way
// this kind of check must not break — it reads a smaller product, reports PASS,
// and leaves no failing assertion to notice.
export const KNOWN_SOURCES = {
  approval: true,
  dedupe_candidate: true,
  task: true,
  brief_item: true,
  conversation_claim: true,
  customer_waiting: true,
  lead_response: true,
  deal_at_risk: true,
  meeting: true,
  relationship_decay: true,
  failed_approval: true,
  dsr: true,
  sync_health: true,
  capture_health: true,
  ai_work_health: true,
  bounce: true,
  undelivered: true,
  automation_run: true,
  notice: true,
  introduction_request: true,
  // A group of routine decisions, which names no single record.
  batch: true,
} as const;

function knownSource(
  source: WorklistItem["source"],
): source is keyof typeof KNOWN_SOURCES {
  return source in KNOWN_SOURCES;
}

// How much of one kind of work the page is carrying, for the pill that narrows
// to it.
//
// A count is drawn only when it is EXACT. `FilterPills` treats an absent count
// as "no number" rather than as a zero, and that is the honest answer for a
// category read to its bound: the server knows a floor, not a total, and a
// floor printed as a count is a wrong number rather than a missing one.
//
// The `all` pill counts every category, because "all" is not a category and has
// no row of its own.
export function pillCount(
  day: Worklist,
  filter: WorklistFilter,
): number | undefined {
  if (filter === "all") {
    return day.counts.some((count) => count.more_available)
      ? undefined
      : day.counts.reduce((total, count) => total + count.considered, 0);
  }
  const found = day.counts.find((count) => count.category === filter);
  if (!found || found.more_available) {
    return undefined;
  }
  return found.considered;
}

// What the page is NOT showing, in one line.
//
// The queue is a cut, and before this the reader had no way to tell a finished
// day from a truncated one — a full first page read as an empty backlog. The
// line is drawn only when there IS a difference to report: on a day the page
// carries whole, saying "12 of 12" is noise.
//
// It reads only the cut the reader is LOOKING at. `considered` is snapshotted
// before the category narrowing, so on a filtered page the other categories keep
// their full figures and contribute nothing shown — summing them all would
// answer "5 of 35" on a page that is showing every one of the five things the
// reader asked for, and conflate "you filtered this out" with "this did not
// fit".
//
// Where a source was read to its bound the total is a floor, and the sentence
// says a source has more rather than printing a number it cannot stand behind.
export function completenessText(
  day: Worklist,
  filter: WorklistFilter,
  t: T,
  locale: Locale,
  // How many rows are on screen NOW. `counts[].shown` describes one response
  // page, and the reader can have walked past several — reading it after a
  // "show more" would report the first page's rows over the whole day's
  // candidates and call a growing list incomplete for ever.
  loaded?: number,
): string | null {
  const counted =
    filter === "all"
      ? day.counts
      : day.counts.filter((count) => count.category === filter);
  const shown =
    loaded ?? counted.reduce((total, count) => total + count.shown, 0);
  const considered = counted.reduce(
    (total, count) => total + count.considered,
    0,
  );
  const bounded = counted.filter((count) => count.more_available).length;
  if (shown >= considered && bounded === 0) {
    return null;
  }
  if (bounded > 0) {
    // No fraction: the figure it would divide by is a floor, and "200 of 200
    // shown · 1 source has more" contradicts itself in one sentence.
    return t("worklist.completeness.bounded", {
      shown: formatNumber(shown, locale),
      sources: formatNumber(bounded, locale),
    });
  }
  return t("worklist.completeness", {
    shown: formatNumber(shown, locale),
    considered: formatNumber(considered, locale),
  });
}
