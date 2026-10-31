import { sourceName } from "./worklist.sources";

export { sourceName } from "./worklist.sources";

import { floorFigure } from "../../format/figure";
import { formatNumber } from "../../format/format";
import type { Locale, useT } from "../../i18n";
import { isMessageKey, translatePlural } from "../../i18n";
import { aiTaskLabel } from "../../lib/ai-task-label";
import {
  BRIEF_PARAM,
  COMPOSE_PARAM,
  THREAD_PARAM,
} from "../../lib/contactpage.address";
import { ENTITY, recordRoute } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { splitStagedSummary } from "../../lib/stagedsummary";
import { caseHref } from "../privacy/privacy.caselink";
import { connectionName, connectionTitleKey } from "./worklist.connection";
import { nonBuyerRequest } from "./worklist.eyebrow";
import type { Worklist, WorklistItem } from "./worklist.queries";

export {
  dealFactsText,
  phrasedReasons,
  reasonText,
  whenText,
} from "./worklist.copy.reasons";

export type T = ReturnType<typeof useT>;

// The queue's words.
//
// The server sends facts and closed vocabularies; every sentence on the page is
// written here, in the reader's own language. That split is why the ranking can
// be explained at all: a reason composed server-side would reach a German
// reader in English, so what travels is `{kind, value}` and the phrase is the
// client's to write.

// Which record an item or a receipt points at, as a router address. Through
// `recordRoute`, which is the one place the product decides whether a
// typed reference off the wire may be linked at all — the approval undo and the
// notification centre ask it too. A switch written here would be a second
// spelling of the record types, and the reader it sent to a page that does not
// exist would have no way of telling which copy was wrong.
export function subjectHref(
  item: Pick<WorklistItem, "subject">,
): string | undefined {
  const route = recordRoute(item.subject?.type, item.subject?.id);
  return route === undefined ? undefined : routeHash(route);
}

// Sources without record pages link to their existing work surface.
const SOURCE_QUEUE: Partial<Record<WorklistItem["source"], string>> = {
  dsr: routeHash(settingsHref("privacy")),
  capture_health: routeHash(settingsHref("connections")),
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
  // The AI page, whose read is the one every seeded role holds — so this
  // answers for a rep as well as an operator rather than routing most readers
  // into a refusal. The catalog splits it into models/automations/usage/
  // model-calls; this follows when the screen does, not before.
  automation_run: routeHash(settingsHref("models")),
  // The same page answers for the AI work that a rule set off, for the same
  // reason and with the same limit.
  ai_work_health: routeHash(settingsHref("models")),
};

// The address a row's headline links to: its record where it has one, the
// queue that owns it where it does not, and nothing at all for a row that is
// neither — a system condition fixed on a settings screen the card does not
// pretend to know.
export function rowHref(item: WorklistItem): string | undefined {
  return (
    askHref(item) ??
    caseQueueHref(item) ??
    subjectHref(item) ??
    SOURCE_QUEUE[item.source]
  );
}

// A subject request goes to the case, not merely to the queue that holds it.
//
// The row already carried the queue's address, and it was one step short: the
// officer arrived at twenty rows with nothing saying which one they had been
// sent to read, and the case they wanted was as likely to be below the fold as
// on screen. The row's `id` IS the case id, so the address can name it
// (privacy.caselink.ts opens it on arrival).
//
// Still no verb. The queue answers these and this page cannot, so the row says
// where the case is and stops there — the difference between telling somebody
// where a room is and claiming to have opened the door, which is the rule the
// table below already follows.
export function caseQueueHref(item: WorklistItem): string | undefined {
  return item.source === "dsr" ? caseHref(item.id) : undefined;
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
    item.subject?.type !== "contact"
  ) {
    return undefined;
  }
  return routeHash({ screen: "contacts", id: item.subject.id, id2: "network" });
}

// Where the row's suggested step leads.
//
// The record the thread belongs to, not the message. An activity is a timeline
// entry with no page of its own (`lib/entity.ts` says so), so
// `#/activities/<id>` would be a control that goes nowhere.
//
// A `draft_reply` row whose subject is a CONTACT opens the composer, through
// `?compose=reply` on their record (contactpage.address.ts COMPOSE_PARAM). Everything
// else lands on the record itself, and its verb says so.
//
// WHY ONLY A CONTACT. The composer lives on the contact page and drafts to the
// contact, choosing its transport from their own reachability. A deal or an
// company has no composer to open, so a link claiming to draft there would
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
// Both end at a contact's composer, and which of the two the server chose
// reaches the address as well as the label: `draft_reply` names the message it
// answers, `draft_email` names none because it is opening a conversation.
//
// `open_meeting_brief` is the third, and it lands somewhere else entirely: the
// brief is read as `?prep=<activity>` on the CONTACT's record, so the address
// needs an id the subject does not carry. The server sends it as `with_contact`,
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
 * and the contact says whose record it opens on — the brief is not a page of its
 * own. A row missing either names nothing openable and draws no control, which
 * is the same promise every other verb here makes about its own operand.
 */
function briefHref(item: WorklistItem): string | undefined {
  const meeting = item.move?.activity_id;
  if (!meeting || !item.with_contact) {
    return undefined;
  }
  const contact = routeHash(ENTITY.contact.route(item.with_contact));
  return `${contact}?${BRIEF_PARAM}=${meeting}`;
}

export function moveHref(item: WorklistItem): string | undefined {
  // Older servers attached a deal move to its task too. Keep the task action
  // consistent while client and server versions can differ.
  if (item.source === "task") return undefined;
  const move = item.move;
  if (!move || !NAVIGABLE_MOVES.has(move.action) || !moveIsComplete(move)) {
    return undefined;
  }
  if (move.action === "open_meeting_brief") {
    return briefHref(item);
  }
  const record = subjectHref(item);
  if (move.action === "open_task") return record;
  if (!record || item.subject?.type !== "contact") {
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
    (move.action === "draft_reply" || move.action === "draft_email") &&
    moveIsComplete(move) &&
    item.subject?.type === "contact"
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
  if (item.move?.action === "open_meeting_brief") {
    return t("worklist.verb.open_meeting_brief");
  }
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

function batchCause(item: WorklistItem, t: T): string {
  if (item.ai_task) return aiTaskLabel(item.ai_task, t, item.batch?.label);
  return (
    item.batch?.label ??
    connectionName(item, t) ??
    t("worklist.batch.unnamedCause")
  );
}

// One item's headline.
//
// The server sends a sentence where it HAS one — an approval summary composed
// at staging time, a deal's own name, a message's subject. Where it has none
// the sentence would have to be invented, and the client writes it from the
// source instead.
export function briefSubjectLabel(item: WorklistItem): string | undefined {
  return item.source === "brief_item"
    ? item.subject?.label || undefined
    : undefined;
}

export function briefContextTitle(item: WorklistItem): string | undefined {
  const subject = briefSubjectLabel(item);
  const title = item.title?.trim();
  return subject &&
    title &&
    title !== subject &&
    title !== item.verdict?.line.trim()
    ? title
    : undefined;
}

// An approval's title is the staged summary: its sentence, then raw field
// pairs. The row reads the sentence; one with none falls to the source title.
function readableTitle(item: WorklistItem): string | undefined {
  if (item.source !== "approval" || !item.title) return item.title;
  return splitStagedSummary(item.title).sentence;
}

export function itemTitle(
  item: WorklistItem,
  t: T,
  locale: Locale,
  subjectNamed = false,
): string {
  // A group names itself by what it holds and how much: "43 likely automated
  // senders" is the whole row, and a reader decides whether to open it from
  // that sentence alone.
  if (item.source === "brief_item" && !item.title && item.subject?.label) {
    return item.subject.label;
  }
  if (item.batch) {
    const { count: size, at_least } = item.batch;
    const count = floorFigure(formatNumber(size, locale), !!at_least, size);
    // An incident names WHAT is broken; a hygiene group names its kind.
    //
    // From `label`, never from `cause`. The cause is the identity the group was
    // formed on and reads like one — interpolating it printed
    // "automation_run:01a0…-… failed 12 times" at a rep, which names nothing
    // they can act on and cannot be told from a bug. A group whose lane minted
    // no name falls back to the generic phrase rather than to the identity.
    // Typed as a plural base, so a batch kind the catalog carries no pair for
    // fails the build; only the incident sentence reads `cause`.
    const base = `worklist.batch.${item.batch.key}` as const;
    const cause = batchCause(item, t);
    // A kind from a newer server has no pair here: its own label, else the
    // generic group name, rather than a lookup that throws.
    if (!isMessageKey(`${base}_other`)) {
      return item.batch.label ?? t("worklist.untitled.batch");
    }
    return translatePlural(locale, base, item.batch.count, { count, cause });
  }
  const named = subjectNamed ? undefined : item.subject?.label;
  const sentence = readableTitle(item);
  if (sentence) {
    // A title that names no record, on a row that HAS one, gets the record's
    // name beside it. Eight rows reading "Follow up with the new lead" cannot
    // be told apart or put in order, and the lead's name is already on the row
    // — it was only the sentence that discarded it.
    return named && !sentence.includes(named)
      ? `${sentence} · ${named}`
      : sentence;
  }
  if (!knownSource(item.source)) {
    return t("worklist.untitled.generic");
  }
  const title = t(
    nonBuyerRequest(item)
      ? "worklist.source.awaiting_reply"
      : (connectionTitleKey(item) ??
          (`worklist.untitled.${item.source}` as const)),
  );
  return named ? `${title} · ${named}` : title;
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

// Through the same known-source check the titles use, so a source this build
// has never heard of is described generically rather than printed as its own
// identifier — a reader must never be shown `ai_work_health` as a noun.
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
  contact_case: true,
  deal_suggestion: true,
  tag_suggestion: true,
  task: true,
  weekly_commitment: true,
  brief_item: true,
  conversation_claim: true,
  customer_waiting: true,
  awaiting_reply: true,
  meeting_follow_up: true,
  lead_response: true,
  deal_at_risk: true,
  meeting: true,
  meeting_outcome: true,
  relationship_decay: true,
  failed_approval: true,
  dsr: true,
  notice_case: true,
  capture_health: true,
  ai_work_health: true,
  bounce: true,
  undelivered: true,
  automation_run: true,
  notice: true,
  introduction_request: true,
  domain_question: true,
  batch: true,
} as const;

function knownSource(
  source: WorklistItem["source"],
): source is keyof typeof KNOWN_SOURCES {
  return source in KNOWN_SOURCES;
}
