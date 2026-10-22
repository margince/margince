import type { ReactNode } from "react";
import { EmailReference } from "../../../design-system/emailreference";
import {
  contactsOn,
  mergeContacts,
  namesOf,
  withWhom,
} from "../../../design-system/participants";
import {
  calendarDaysBetween,
  formatDateAbbrev,
  formatNumber,
} from "../../../format/format";
import { translatePlural, type useLocale, type useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { EXCHANGE_KINDS, type ExchangeKind, isExchange } from "./exchangekinds";
import { isContactBy, lastContactAt, waitingSince } from "./lastcontact";
import type { SpineCommercial, SpineSource, Stop } from "./spine";

export type Ctx = {
  t: ReturnType<typeof useT>;
  // The instant the read describes. Today's marker is dated from it rather
  // than from the machine clock. The line sits where the rest of the card is
  // measured from, and does not drift while a tab is left open.
  asOf: string;
  locale: ReturnType<typeof useLocale>["locale"];
  zone: string;
  commercial?: SpineCommercial | null;
  nameOf?: (entityType: string, entityId: string) => string | undefined;
  // Opens a cited message in the record page's own drawer. Optional, so a
  // story or a host with no drawer draws the subject as plain text rather than
  // a control that leads nowhere.
  onOpenEmail?: (activityId: string) => void;
};

// The waiting itself, as its own stop.
//
// It is the only stop with no record behind it, and the reason this component
// exists. An account nobody has heard from is a fact no row on the page states,
// because it is the absence of rows. The count is calendar days from the last
// exchange, which is what a reader means by "days".
export function silence(view: SpineSource, ctx: Ctx): Stop | undefined {
  const since = waitingSince(view);
  if (!since) {
    return undefined;
  }
  const days = calendarDaysBetween(new Date(since), new Date(view.as_of));
  if (days < 1) {
    return undefined;
  }
  // Never having written at all is a different fact from having gone quiet.
  // The two lead to different moves: one relationship has not started, the
  // other has stalled.
  const everReplied = Boolean(view.last_inbound_at);
  return {
    key: "silence",
    tone: "gap",
    when: translatePlural(ctx.locale, "co.spine.days", days, {
      count: formatNumber(days, ctx.locale),
    }),
    title: everReplied
      ? ctx.t("co.spine.quietSince")
      : ctx.t("co.spine.neverReplied"),
    detail: view.health?.single_threaded
      ? ctx.t("co.spine.singleThreaded")
      : undefined,
  };
}

export function on(at: string, ctx: Ctx): string {
  return formatDateAbbrev(at, ctx.locale, ctx.zone);
}

// How many conversations the thread draws before the silence.
//
// Three is what fits above the fold beside the gap and what is left ahead. A
// record with more history keeps its older conversations on the history tab.
// The reader is told the count rather than left to assume three is all.
const PAST_STOPS = 3;

// One conversation, as the thread tells it. `key` is the identity it was
// grouped under and outlives its newest message. React keeps the same list
// item when a reply arrives and moves the conversation's date.
type Exchange = {
  readonly key: string;
  readonly at: string;
  readonly subject: string;
  readonly kind: ExchangeKind;
  readonly count: number;
  // The newest message's id, when this conversation is mail the reader may
  // read. That message dates and titles the thread, so it is the one a reader
  // pressing the stop means. Undefined for a call or a meeting, and for a
  // message whose content is not theirs. `email_summary` answers both: it is
  // absent for every kind but email, and withheld when the words are not
  // theirs.
  readonly emailId?: string;
  // Who was on it, in the order the conversation introduced them. A mail or a
  // call with no name beside it is a subject line and nothing a reader can act
  // on. The question is who they have to go back to.
  readonly contacts: readonly string[];
  // Which way the conversation's latest message went. The conversation as a
  // whole has no direction; its last message does, and that is what says who
  // owes whom.
  readonly direction?: "inbound" | "outbound" | null;
  // The colleague who held the meeting, named. Undefined on everything that is
  // not a meeting, and on a meeting nobody was recorded as hosting.
  readonly host?: string;
};

// What was said here, oldest conversation first.
//
// Dated off the exchanges themselves rather than off `health.last_meeting_at`.
// Those are two different facts: a meeting date beside an email's subject reads
// as a meeting about that subject. Falls back to the meeting/outbound date only
// when the timeline is withheld or empty. A date with no subject still starts
// the thread, and without it the gap below would measure from nothing.
export function pastStops(view: SpineSource, ctx: Ctx): Stop[] {
  const conversations = exchanges(view, ctx);
  // A cut page has history the thread never saw, whatever it holds. A page of
  // tasks and notes can name two conversations and hide twenty older ones. A
  // thread that drew "the whole history" would be wrong in a way a reader
  // cannot check.
  const cut = view.activities?.page?.has_more === true;
  if (conversations.length === 0) {
    const at = lastContactAt(view);
    const stops: Stop[] = at
      ? [
          {
            key: "spoke",
            tone: "past",
            when: on(at, ctx),
            title: ctx.t("co.spine.lastSpoke"),
          },
        ]
      : [];
    return cut ? [earlierStop(conversations, 0, view, ctx), ...stops] : stops;
  }
  // Newest conversations, then re-read oldest-first. The thread runs across
  // the card in time order, and the oldest is dropped when a record has more
  // history than the thread draws.
  const shown = conversations.slice(0, PAST_STOPS).reverse();
  const earlier = conversations.length - shown.length;
  // The last exchange, when it is later than every conversation the thread
  // can name. That is a message or meeting with no subject, or one this reader
  // may not see. The gap below counts from it, so it takes the head's label.
  // Otherwise "Last contact" dates one conversation and the gap another.
  const unnamed = unnamedLastWord(view, shown.at(-1), ctx);
  const stops = shown.map((conversation, index) =>
    pastStop(conversation, !unnamed && index === shown.length - 1, ctx),
  );
  if (unnamed) {
    stops.push(unnamed);
  }
  if (earlier > 0 || cut) {
    stops.unshift(earlierStop(conversations, earlier, view, ctx));
  }
  return stops;
}

// The stop for a last exchange the thread cannot name, or undefined when the
// newest drawn conversation is the last exchange.
function unnamedLastWord(
  view: SpineSource,
  newest: Exchange | undefined,
  ctx: Ctx,
): Stop | undefined {
  const spoke = lastContactAt(view);
  if (!spoke || !newest) {
    return undefined;
  }
  const said = Date.parse(spoke);
  const drawn = Date.parse(newest.at);
  if (Number.isNaN(said) || Number.isNaN(drawn) || said <= drawn) {
    return undefined;
  }
  return {
    key: "spoke",
    tone: "past",
    when: on(spoke, ctx),
    title: ctx.t("co.spine.lastSpoke"),
  };
}

// The conversations the thread did not draw, as one stop.
//
// It counts what the read sent, and the read sends one capped page of the
// timeline. Past that page, an exact count would disagree with the history tab.
// So a cut page says "more" instead, and drops its date with the count. The
// oldest conversation on a cut page is not where the record began.
function earlierStop(
  conversations: readonly Exchange[],
  earlier: number,
  view: SpineSource,
  ctx: Ctx,
): Stop {
  const cut = view.activities?.page?.has_more === true;
  const folded = conversations.slice(conversations.length - earlier);
  const latest = conversations.at(-1);
  return {
    key: "earlier",
    tone: "past",
    when: cut || latest === undefined ? "" : on(latest.at, ctx),
    title: cut
      ? ctx.t("co.spine.earlierMore")
      : // Through the catalog's own plural rule rather than an if: which
        // categories a language has is the catalog's to know. A branch here
        // would carry English's two into every locale.
        translatePlural(ctx.locale, "co.spine.earlier", earlier, {
          count: formatNumber(earlier, ctx.locale),
        }),
    // Who those conversations were with. A count on its own says the record
    // has a history and nothing about it. The names tell a reader whether the
    // history is with the contact they are about to write to.
    detail: withWhom(
      folded.reduce<readonly string[]>(
        (contacts, conversation) =>
          mergeContacts(contacts, conversation.contacts),
        [],
      ),
      ctx.t,
      ctx.locale,
    ),
  };
}

// A conversation's own stop. The newest one keeps "You last spoke" as its
// title. The gap below is counted from it, and a reader has to see what the
// waiting is waiting on. The older ones lead with their subject. They are what
// happened, and one label repeated down the thread would say nothing.
function pastStop(conversation: Exchange, newest: boolean, ctx: Ctx): Stop {
  const relation = relationLine(conversation, ctx);
  // A mail conversation names its message the way every other citation of one
  // does, and opens it. A call or a meeting keeps the plain subject. There is
  // no message behind those to open, and an envelope beside one would claim a
  // transport nobody checked.
  const subject =
    conversation.emailId && ctx.onOpenEmail ? (
      <EmailReference
        subject={conversation.subject}
        onOpen={openEmail(conversation.emailId, ctx.onOpenEmail)}
      />
    ) : (
      conversation.subject
    );
  return {
    key: `said-${conversation.key}`,
    tone: "past",
    when: on(conversation.at, ctx),
    title: newest ? ctx.t("co.spine.lastSpoke") : subject,
    // The newest stop's title is the label the gap below counts from. Its
    // subject moves into the second line, and the relation joins it there.
    detail: newest ? saidLines(subject, relation) : relation,
  };
}

// Bound here rather than in the JSX so the closure captures the narrowed id.
// `conversation.emailId` is optional, and a callback reading it again would
// read a `string | undefined`.
function openEmail(
  activityId: string,
  onOpenEmail: (activityId: string) => void,
): () => void {
  return () => onOpenEmail(activityId);
}

// The record's conversations, newest first, each folded to one entry.
//
// `thread_key` is capture's own conversation id and makes a thread a thread.
// A note or a hand-logged call carries none, so those fall back to their
// subject. Grouping by subject alone would merge two unrelated "Re: Update"
// exchanges: acceptable as a fallback for rows with no better key, wrong as
// the rule.
function exchanges(view: SpineSource, ctx: Ctx): Exchange[] {
  const asOf = Date.parse(view.as_of);
  const conversations = new Map<string, Exchange>();
  for (const entry of view.activities?.data ?? []) {
    const subject = bareSubject(entry.subject);
    // `isExchange` again only to narrow the kind; `isContactBy` is the rule.
    if (!isExchange(entry.kind) || !isContactBy(entry, asOf) || !subject) {
      continue;
    }
    // The two key spaces never meet: a provider whose thread ids look like our
    // own fallback would otherwise fold an unrelated conversation into it.
    const key = entry.thread_key
      ? `thread:${entry.thread_key}`
      : // Lowercased against the invariant locale, never the machine's: this is
        // a grouping key rather than a rendered value. A Turkish browser
        // folding "I" to "ı" would file one record's conversations apart from
        // every other reader's.
        `subject:${subject.toLowerCase()}`;
    const seen = conversations.get(key);
    const contacts = namesOf(contactsOn(entry.links, ctx.nameOf));
    // The list arrives newest-first, so the first row of a conversation is its
    // latest message. The thread shows it at that date, and under that subject
    // when the conversation was renamed mid-thread.
    conversations.set(
      key,
      seen
        ? {
            ...seen,
            count: seen.count + 1,
            contacts: mergeContacts(seen.contacts, contacts),
          }
        : {
            key,
            at: entry.occurred_at as string,
            subject,
            kind: entry.kind,
            count: 1,
            contacts,
            direction: entry.direction,
            // Set from the first row of a conversation, which this list orders
            // newest-first. The date and subject above come from that row, so
            // the stop opens the message it is showing.
            emailId: openableEmailId(entry),
            host: entry.host_user_id
              ? ctx.nameOf?.("user", entry.host_user_id)
              : undefined,
          },
    );
  }
  return [...conversations.values()];
}

/**
 * The id of a message this stop can open, or nothing.
 *
 * Both conditions come off `email_summary` rather than off the kind string.
 * The server sets it only for `kind=email`, and a message outside the reader's
 * audience carries it with `display_status: withheld`. Opening one of those
 * would draw a placeholder, and the reader learns citations do not work.
 */
function openableEmailId(entry: {
  id: string;
  email_summary?: { display_status: string } | null;
}): string | undefined {
  const summary = entry.email_summary;
  if (!summary || summary.display_status === "withheld") {
    return undefined;
  }
  return entry.id;
}

// What the exchange was, and between whom: "Email to Frédéric de Gombert",
// "Call from Lena Fischer", "Meeting with Lena Fischer and 2 others".
//
// The kind and the direction are two facts a subject line carries neither of.
// They are the two a reader needs to know whose move it is. A conversation
// with nobody nameable still says what kind it was, which is always true.
function relationLine(conversation: Exchange, ctx: Ctx): string {
  const what =
    conversation.count > 1
      ? ctx.t("co.spine.exchangeCount", {
          count: formatNumber(conversation.count, ctx.locale),
        })
      : ctx.t(EXCHANGE_KINDS[conversation.kind]);
  const who = withWhom(conversation.contacts, ctx.t, ctx.locale);
  // A meeting we know the host of names both sides, the one exchange where the
  // record can: "Lena Fischer met Frédéric de Gombert". On an account several
  // colleagues work, who was in the room is half of what happened.
  //
  // Meetings only. `host_user_id` is a column on every activity row, and a mail
  // carrying one was still sent rather than held. Reading "met" over an email
  // would rewrite what happened.
  if (conversation.kind === "meeting" && conversation.host) {
    return who
      ? ctx.t("co.spine.said.met", { host: conversation.host, who })
      : ctx.t("co.spine.said.held", { host: conversation.host });
  }
  if (!who) {
    return what;
  }
  return ctx.t(preposition(conversation), { what, who });
}

// Which word joins the exchange to the contacts on it.
//
// A meeting is with everyone in it however it was arranged, so it never takes
// a side. Everything else follows the message: one we sent went to them, and
// one they sent us came from them. An exchange with no recorded direction says
// "with" rather than guessing a side that decides who owes a reply.
function preposition(conversation: Exchange): MessageKey {
  if (conversation.kind === "meeting" || !conversation.direction) {
    return "co.spine.said.with";
  }
  return conversation.direction === "inbound"
    ? "co.spine.said.from"
    : "co.spine.said.to";
}

// The newest stop's detail: what was said, then what kind of exchange said it.
//
// Two lines rather than one joined sentence. The subject is the half a reader
// recognises, and the relation says whose move it is. Joined by a separator,
// the subject reads as one long caption and a scanning reader cannot find it.
// `what` is a node rather than a string. A mail conversation names its message
// as a citation that opens, and a call names its subject as text.
function saidLines(what: ReactNode, relation: string): ReactNode {
  if (!what) {
    return relation;
  }
  return (
    <>
      <span className="co-spine-said">{what}</span>
      <span className="co-spine-relation">{relation}</span>
    </>
  );
}

// A subject with its reply and forward markers taken off. "Re: Kickoff" and
// "Kickoff" are then one conversation when neither row was threaded by
// capture.
//
// Repeated markers are stripped to the end. A replied and forwarded chain
// arrives as "Re: Fwd: Kickoff", and taking one prefix off would file it apart
// from its conversation. Only the ASCII prefixes every mail client writes: a
// localized "AW:" that slips through costs one extra stop, the cheap error.
//
// Returns "" for a subject that is absent, blank, or nothing but markers.
// Those are not conversations the thread can name. One empty key would merge
// unrelated calls into a stop with no title.
function bareSubject(subject: string | null | undefined): string {
  let bare = (subject ?? "").trim();
  let stripped = bare.replace(/^(?:re|fwd?)\s*:\s*/i, "").trim();
  while (stripped !== bare) {
    bare = stripped;
    stripped = bare.replace(/^(?:re|fwd?)\s*:\s*/i, "").trim();
  }
  return bare;
}
