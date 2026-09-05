// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record as ONE thread, in time order: what has happened, the silence
// since, and what is coming.
//
// The page this replaced printed the same facts three times — a header strip,
// a facts box and a deal row all saying "1 open · closes 20/10/2026" — at one
// weight, so nothing led and a reader had nowhere to look first. Worse, the
// fact that actually decides what to do next was not on the page at all: not
// "there is a deal" and not "there is a meeting", but that SEVEN DAYS have
// passed since the only conversation and nobody has replied.
//
// A gap is invisible in a list and unmissable on a thread. That is the whole
// reason this is a spine rather than a fourth card: the rule below draws the
// waiting itself, at a size that reads before the words around it.

import type { ReactNode } from "react";

import { useRecordZone } from "../../app/recordzone";
import { EmailReference } from "../../design-system/emailreference";
import { PanelBody } from "../../design-system/panel";
import {
  mergePeople,
  peopleOn,
  withWhom,
} from "../../design-system/participants";
import {
  calendarDaysBetween,
  formatDateAbbrev,
  formatDayMonth,
  formatNumber,
} from "../../format/format";
import { translatePlural, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import "./spine.css";

/**
 * What the thread reads, and nothing else.
 *
 * Named structurally rather than as a record type, because that is the kit's
 * own test for what belongs in it: the spine holds no opinion about WHICH
 * record it is drawing. An account, a contact and a lead all have a last word,
 * a silence and things dated ahead, and the arithmetic between two dates is
 * the same arithmetic on all three.
 */
export type SpineSource = {
  // The instant the read describes. Every "how long ago" is measured from
  // here rather than from now, so the thread agrees with the cards beside it.
  as_of: string;
  last_inbound_at?: string | null;
  last_outbound_at?: string | null;
  // Every optional field admits null as well as absent, because the wire
  // sends both and they mean the same thing here: a fact the read did not
  // establish. A narrower shape would compile against the vanilla schema and
  // fail against a composed one that spells the same field nullable.
  health?: {
    last_meeting_at?: string | null;
    single_threaded?: boolean | null;
  } | null;
  next_steps?: {
    data: readonly {
      activity_id: string;
      subject: string;
      due_at?: string | null;
      overdue?: boolean | null;
    }[];
  } | null;
  activities?: {
    data: readonly {
      // The row's own id, which is what a stop opens when the conversation is
      // mail. Required rather than optional: every activity has one, and a
      // shape that admitted its absence would let a caller hand this thread
      // rows it could name and never open.
      id: string;
      kind: string;
      subject?: string | null;
      occurred_at?: string | null;
      // Present exactly when the row is an email, which is how a stop tells a
      // mail conversation from a call without reading the kind string. It also
      // carries whether the words are this reader's: a withheld message names
      // itself and opens nothing.
      email_summary?: { display_status: string } | null;
      // Capture's own conversation id. What makes a thread a thread — a note
      // or a hand-logged call carries none, and those fall back to their
      // subject.
      thread_key?: string | null;
      // What the message is filed against. The people among them are who was
      // ON it, which is the half of "what happened" a subject line does not
      // carry.
      links?: readonly { entity_type: string; entity_id: string }[];
      // Which way it went. A mail a reader SENT and one they were sent are
      // different facts about the same subject line, and the second is the one
      // that owes a reply.
      direction?: "inbound" | "outbound" | null;
      // Meeting only: the colleague who held it. The one field on an activity
      // that names OUR side of an exchange — a mail carries the contact it was
      // with and nothing about the mailbox behind it.
      host_user_id?: string | null;
    }[];
    // Whether the read sent a cut page. A thread that drew three of four
    // conversations can count the rest; one drawn from a cut page cannot, and
    // an exact number a reader could check against the history tab and find
    // wrong is worse than "more".
    page?: { has_more?: boolean | null } | null;
  } | null;
};

/**
 * The date the record's money is next expected to land, for a record that has
 * money at all.
 *
 * A separate prop rather than a field of the source: a contact has no
 * pipeline, and a source shape that carried one would invite a caller to
 * invent a date for a record that cannot have it.
 *
 * The DATE and nothing else. What the pipeline is worth is the readings row's
 * own figure, and the thread printed the same total a second time directly
 * under it — one fact, two places, and a reader reconciling them.
 */
export type SpineCommercial = {
  next_close_on?: string | null;
};

// One stop on the thread. `tone` is what the dot and the rule say about it,
// never decoration: `past` is filed, `gap` is the waiting itself, `ahead` is
// dated and has not happened.
type Stop = {
  key: string;
  tone: "past" | "gap" | "ahead" | "overdue" | "now";
  when: string;
  title: ReactNode;
  detail?: ReactNode;
};

/**
 * RecordSpine is the record's story as one horizontal thread.
 *
 * It reads the same payload the cards below it read and adds nothing — every
 * stop is a record, a date, or the arithmetic between two dates. What it adds
 * is ORDER: the reader sees the last real contact, how long ago that was, and
 * what is dated ahead, in one left-to-right glance.
 */
export function RecordSpine({
  source,
  commercial,
  nameOf,
  onOpenEmail,
}: Readonly<{
  source?: SpineSource;
  // Opens a cited message. The record page owns its one drawer, so the thread
  // asks rather than mounting a second one behind the first.
  onOpenEmail?: (activityId: string) => void;
  // What a linked record is called. Handed in, because the names live in the
  // sections around this one and the thread holds no read of its own: a
  // conversation the thread cannot name a person for says the kind alone
  // rather than an id nobody can read.
  nameOf?: (entityType: string, entityId: string) => string | undefined;
  // Null as well as absent: a record whose commercial reading was withheld
  // sends null, and it means the same as a record that has no money at all —
  // there is no last stop to price.
  commercial?: SpineCommercial | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  if (!source) {
    return null;
  }
  const stops = spineStops(source, {
    t,
    locale,
    zone,
    commercial,
    nameOf,
    onOpenEmail,
    asOf: source.as_of,
  });
  if (stops.length === 0) {
    return null;
  }
  // The gap takes half again the width of the stops either side of it. It is
  // the one stop with no record behind it, and on a horizontal axis the WIDTH
  // is the waiting — a silence drawn the same width as the meeting before it
  // says the two took the same amount of time.
  const columns = stops
    .map((stop) =>
      // The marker is a line rather than a stop, so it takes only the width
      // of the word on it.
      todayColumn(stop),
    )
    .join(" ");
  return (
    <PanelBody className="co-spine">
      {/* The axis runs ACROSS, not down: a reader takes the account's shape —
          what happened, how long nothing did, what is dated next — in one
          left-to-right glance, which is the reading a stacked list cannot
          give. It scrolls rather than wraps, because a time axis that wrapped
          onto a second line would put a later stop to the left of an earlier
          one. */}
      <div className="co-spine-scroll">
        <ol className="co-spine-track" style={{ gridTemplateColumns: columns }}>
          {stops.map((stop) => (
            <li
              key={stop.key}
              className={`co-spine-stop co-spine-${stop.tone}`}
            >
              <span className="co-spine-when t-caption">{stop.when}</span>
              {/* The rule is this stop's own stretch of the axis, so a stop
                  restyles its segment without the seams a border between
                  siblings would leave. The dot rides on it. */}
              <span className="co-spine-rail" aria-hidden="true">
                <span className="co-spine-dot" />
              </span>
              {/* A stop with nothing to say says nothing: today's marker is a
                  line through the axis, and an empty title element would put a
                  blank row into the thread's own list of what happened. */}
              {stop.title ? (
                <span className="co-spine-title">{stop.title}</span>
              ) : null}
              {stop.detail && (
                <span className="co-spine-detail t-caption">{stop.detail}</span>
              )}
            </li>
          ))}
        </ol>
      </div>
    </PanelBody>
  );
}

// The narrowest a stop may be drawn. A date is set on one line — broken across
// two it reads as two moments — so under any less than this the dates run into
// each other and the row becomes one smear of digits. At the floor the track
// outgrows its card and the thread scrolls, which is what this axis already
// does with more history than fits; crushing the stops is what it may not do.
const STOP_FLOOR = "6.5rem";

// How much of the axis a stop takes. The gap takes half again the stops either
// side of it — on a horizontal axis the width IS the waiting — and today's
// marker takes only its own label, because it is a line and not a span of
// time.
function todayColumn(stop: Stop): string {
  if (stop.tone === "now") {
    // Its label's width and not a share of the axis. `auto` reads the stop's
    // MINIMUM contribution, which every stop declares as zero so a long
    // subject cannot widen the thread — so once the track outgrew its card the
    // marker was the one column that collapsed, and today vanished off an axis
    // whose whole reading is what falls either side of it.
    return "max-content";
  }
  return stop.tone === "gap"
    ? `minmax(${STOP_FLOOR}, 1.5fr)`
    : `minmax(${STOP_FLOOR}, 1fr)`;
}

type Ctx = {
  t: ReturnType<typeof useT>;
  // The instant the read describes. Today's marker is dated from it rather
  // than from the machine clock, so the line sits where the rest of the card
  // is measured from and does not drift while a tab is left open.
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

// The thread, oldest first: the conversations that have happened, the silence
// since the last of them, then what is dated ahead.
//
// One stop per CONVERSATION, not per message and not per row of the timeline.
// A reader arriving at a record they do not remember asks what was going on
// here, and a single stop naming the newest email answered that with one line
// out of a story — the kickoff that explains the account sat three scrolls
// down under the day's suggestions. Grouping is what keeps this a thread
// rather than the history tab: six emails in two conversations are two stops,
// and the tab is still where an individual message is read.
function spineStops(view: SpineSource, ctx: Ctx): Stop[] {
  const stops: Stop[] = [...pastStops(view, ctx)];
  const waiting = silence(view, ctx);
  if (waiting) {
    stops.push(waiting);
  }
  for (const ahead of aheadStops(view, ctx)) {
    stops.push(ahead);
  }
  return withToday(stops, ctx);
}

// Today, marked on the axis.
//
// Not a stop: nothing happened today, and drawing one would put an event on
// the thread that the record does not contain. It is a LINE through the axis,
// and what it earns is the reading either side of it — everything left of it
// happened, everything right of it is owed. A reader was working that out from
// the dates.
//
// It goes before the first stop that has not happened yet. A slipped
// commitment is dated in the PAST however it is drawn, so it stays on the left
// where its date puts it; only the `ahead` stops sit beyond the line. With
// nothing ahead at all the line closes the axis, which is the true reading: the
// record runs up to today and stops.
function withToday(stops: readonly Stop[], ctx: Ctx): Stop[] {
  if (stops.length === 0) {
    return [...stops];
  }
  const first = stops.findIndex((stop) => stop.tone === "ahead");
  const marker: Stop = {
    key: "today",
    tone: "now",
    // No date on the axis's own date row: the marker is not a moment the
    // record reached, it is where the reader is standing. Its word and its
    // date sit under the line instead, out of the row the record's own dates
    // share.
    when: "",
    title: (
      <>
        <span className="co-spine-now-word">{ctx.t("co.spine.today")}</span>
        {/* Day and month, no year. The marker names TODAY, and the year of
            today is the one date on the axis a reader already knows. */}
        <span className="co-spine-now-date">
          {formatDayMonth(ctx.asOf, ctx.locale, ctx.zone)}
        </span>
      </>
    ),
  };
  if (first === -1) {
    return [...stops, marker];
  }
  return [...stops.slice(0, first), marker, ...stops.slice(first)];
}

// The waiting itself, as its own stop.
//
// It is the only stop with no record behind it, and it is the reason this
// component exists: an account nobody has heard from is a fact no row on the
// page states, because it is the ABSENCE of rows. The count is calendar days
// from the last conversation, which is what a person means by "days".
//
// Never drawn on an account we heard from more recently than we wrote: that
// is a conversation in progress, not a silence.
// The instant the waiting is counted from: the last thing WE sent.
//
// Not the last meeting, which is a different question. An account we met in
// June and wrote to in July has been waiting since July, and counting from
// the meeting would report six weeks of silence that did not happen. The
// meeting date is only the fallback, for an account whose outbound is
// withheld or has none.
//
// One reading, shared by the stop that dates the thread's head and the gap
// that measures from it — two copies let the thread say one date above a gap
// counted from another.
function silenceSince(view: SpineSource): string | undefined {
  return view.last_outbound_at ?? view.health?.last_meeting_at ?? undefined;
}

// Whether they have answered the last thing we sent.
//
// Compared against our own last outbound rather than against whatever
// `silenceSince` fell back to: a reply that came BEFORE our latest message
// does not answer it, and an account whose last word is ours is waiting on
// them however long ago they last wrote.
function silence(view: SpineSource, ctx: Ctx): Stop | undefined {
  // Our own last message, never the meeting `silenceSince` falls back to. A
  // gap is time spent WAITING FOR A REPLY, and a meeting is not something a
  // reply is owed to: an account we met in June and never wrote to is not
  // ignoring us, and drawing "nobody has written back" over it invents a
  // slight that did not happen.
  const since = view.last_outbound_at;
  if (!since || (view.last_inbound_at && view.last_inbound_at > since)) {
    return undefined;
  }
  const days = calendarDaysBetween(new Date(since), new Date(view.as_of));
  if (days < 1) {
    return undefined;
  }
  // Never having written at all is a different fact from having gone quiet,
  // and the two lead to different moves: one is a relationship that has not
  // started, the other one that stalled.
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

// What is dated ahead: the commitments that carry a due date, then the money.
//
// A task already past its date is drawn as overdue rather than ahead — it is
// not "coming", it is late, and a thread that sorted it into the future would
// be the one place on the page that hides it.
function aheadStops(view: SpineSource, ctx: Ctx): Stop[] {
  const stops: Stop[] = [];
  for (const step of (view.next_steps?.data ?? []).slice(0, 2)) {
    if (!step.due_at) {
      continue;
    }
    stops.push({
      key: `step-${step.activity_id}`,
      tone: step.overdue ? "overdue" : "ahead",
      when: on(step.due_at, ctx),
      title: step.subject,
      detail: step.overdue ? ctx.t("co.spine.overdue") : undefined,
    });
  }
  const close = ctx.commercial?.next_close_on;
  if (close) {
    stops.push({
      key: "close",
      tone: "ahead",
      when: on(close, ctx),
      title: ctx.t("co.spine.expectedClose"),
    });
  }
  return stops;
}

function on(at: string, ctx: Ctx): string {
  return formatDateAbbrev(at, ctx.locale, ctx.zone);
}

// The kinds that are an EXCHANGE with the record, each with what a single one
// of it is called. The timeline section is unfiltered — it carries tasks from
// the same table — and a task is something we wrote to ourselves rather than
// something that was said.
//
// A map rather than a set plus a template-literal key: `t` takes a declared
// MessageKey, so writing the key from the kind would put the catalog beyond
// what the compiler can check and let a new kind ship printing its own id.
const EXCHANGE_KINDS = {
  email: "co.spine.kind.email",
  call: "co.spine.kind.call",
  meeting: "co.spine.kind.meeting",
  note: "co.spine.kind.note",
  message: "co.spine.kind.message",
} as const satisfies Record<string, MessageKey>;

type ExchangeKind = keyof typeof EXCHANGE_KINDS;

function isExchange(kind: string): kind is ExchangeKind {
  return kind in EXCHANGE_KINDS;
}

// How many conversations the thread draws before the silence.
//
// Three is what fits above the fold beside the gap and what is left ahead. A
// record with more history than that has its older conversations on the
// history tab, and the reader is told the count rather than left to assume
// three is all there was.
const PAST_STOPS = 3;

// One conversation, as the thread tells it. `key` is the identity it was
// grouped under and outlives its newest message, so React keeps the same list
// item when a reply arrives and moves the conversation's date.
type Exchange = {
  readonly key: string;
  readonly at: string;
  readonly subject: string;
  readonly kind: ExchangeKind;
  readonly count: number;
  // The NEWEST message's id, when this conversation is mail the reader may
  // read. That message is what the thread is dated and titled by, so it is the
  // one a reader pressing the stop means. Undefined for a call or a meeting,
  // and for a message whose content is not theirs — `email_summary` answers
  // both questions, being absent for every kind but email and carrying the
  // withheld status when the words are not the reader's.
  readonly emailId?: string;
  // Who was on it, in the order the conversation introduced them. A mail or a
  // call with no name beside it is a subject line and nothing a reader can act
  // on: the whole question is who they have to go back to.
  readonly people: readonly string[];
  // Which way the conversation's latest message went. The conversation as a
  // whole has no direction; its last message does, and that is what says who
  // owes whom.
  readonly direction?: "inbound" | "outbound" | null;
  // The colleague who held the meeting, named. Undefined on everything that is
  // not a meeting, and on a meeting nobody was recorded as hosting.
  readonly host?: string;
};

// What was actually SAID here, oldest conversation first.
//
// Dated off the exchanges themselves rather than off `health.last_meeting_at`,
// because those are two different facts and the thread states one of them: a
// meeting date beside an email's subject reads as a meeting about that
// subject. Falls back to the meeting/outbound date only when the timeline is
// withheld or empty — a date with no subject is still the start of the thread,
// and losing it would leave the gap below measuring from nothing.
function pastStops(view: SpineSource, ctx: Ctx): Stop[] {
  const conversations = exchanges(view, ctx);
  // A cut page has history the thread never saw, whatever it holds: a page of
  // tasks and notes can name two conversations and hide twenty older ones,
  // and a thread that then drew "the whole history" would be wrong in a way a
  // reader cannot check.
  const cut = view.activities?.page?.has_more === true;
  if (conversations.length === 0) {
    const at = silenceSince(view);
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
  // Newest conversations, then re-read oldest-first: the thread runs across
  // the card in time order, and it is the OLDEST that gets dropped when a
  // record has more history than the thread draws.
  const shown = conversations.slice(0, PAST_STOPS).reverse();
  const earlier = conversations.length - shown.length;
  // Our last word, when it is later than every conversation the thread could
  // name: a message with no subject, or one whose content this reader may not
  // see. The gap below is counted from it, so it takes the head's label and
  // its own date — otherwise the thread dates "You last spoke" at one
  // conversation while the gap beneath counts from another.
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

// The stop for a last word the thread cannot name, or undefined when the
// newest drawn conversation already IS our last word.
function unnamedLastWord(
  view: SpineSource,
  newest: Exchange | undefined,
  ctx: Ctx,
): Stop | undefined {
  const spoke = silenceSince(view);
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
// It counts what the read SENT, and the read sends one capped page of the
// timeline. On a record with more history than that page holds, an exact count
// would be a number the reader can check against the history tab and find
// wrong, so a cut page says "more" instead — and its date is dropped with the
// count, because the oldest conversation on a cut page is not where the record
// began.
function earlierStop(
  conversations: readonly Exchange[],
  earlier: number,
  view: SpineSource,
  ctx: Ctx,
): Stop {
  const cut = view.activities?.page?.has_more === true;
  const folded = conversations.slice(conversations.length - earlier);
  return {
    key: "earlier",
    tone: "past",
    when: cut ? "" : on(conversations[conversations.length - 1].at, ctx),
    title: cut
      ? ctx.t("co.spine.earlierMore")
      : // Through the catalog's own plural rule rather than an if: which
        // categories a language has is the catalog's to know, and a branch here
        // would carry English's two into every locale.
        translatePlural(ctx.locale, "co.spine.earlier", earlier, {
          count: formatNumber(earlier, ctx.locale),
        }),
    // Who those conversations were with. A count on its own says the record
    // has a history and nothing about it; the names are what tell a reader
    // whether the history is with the person they are about to write to.
    detail: withWhom(
      folded.reduce<readonly string[]>(
        (people, conversation) => mergePeople(people, conversation.people),
        [],
      ),
      ctx.t,
      ctx.locale,
    ),
  };
}

// A conversation's own stop. The newest one keeps "You last spoke" as its
// title, because the gap below is counted from it and a reader has to see what
// the waiting is waiting on. The older ones lead with their subject: they are
// what happened, and repeating one label down the thread would say nothing.
function pastStop(conversation: Exchange, newest: boolean, ctx: Ctx): Stop {
  const relation = relationLine(conversation, ctx);
  // A mail conversation names its message the way every other citation of one
  // does, and opens it. A call or a meeting keeps the plain subject: there is
  // no message behind those to open, and drawing an envelope beside one would
  // claim a transport nobody checked.
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
    // The newest stop's title is the label the gap below counts from, so its
    // subject moves into the second line and the relation joins it there.
    detail: newest ? saidLines(subject, relation) : relation,
  };
}

// Bound here rather than in the JSX so the narrowed id is what the closure
// captures — `conversation.emailId` is optional, and a callback reading it
// again would be reading a `string | undefined`.
function openEmail(
  activityId: string,
  onOpenEmail: (activityId: string) => void,
): () => void {
  return () => onOpenEmail(activityId);
}

// The record's conversations, newest first, each folded to one entry.
//
// `thread_key` is capture's own conversation id and is what makes a thread a
// thread; a note or a hand-logged call carries none, so those fall back to
// their subject. Grouping by subject alone would merge two unrelated "Re:
// Update" exchanges — acceptable as a fallback for the rows that have no
// better key, wrong as the rule.
function exchanges(view: SpineSource, ctx: Ctx): Exchange[] {
  const asOf = Date.parse(view.as_of);
  const conversations = new Map<string, Exchange>();
  for (const entry of view.activities?.data ?? []) {
    const subject = bareSubject(entry.subject);
    const at = Date.parse(entry.occurred_at ?? "");
    if (
      !isExchange(entry.kind) ||
      !subject ||
      // Already happened, as of the read the rest of the card describes: an
      // `occurred_at DESC` list sorts a meeting booked for next week to the
      // top, and it has not been said yet. Compared as instants: the same
      // moment is written with any offset, and comparing the strings would
      // read 08:30-02:00 as earlier than 09:00Z when it is ninety minutes
      // later. NaN from a malformed date fails both tests and drops the row,
      // which is the only safe reading of a timestamp nothing can order.
      Number.isNaN(at) ||
      Number.isNaN(asOf) ||
      at > asOf
    ) {
      continue;
    }
    // The two key spaces never meet: a provider whose thread ids look like our
    // own fallback would otherwise merge an unrelated conversation into one it
    // has nothing to do with.
    const key = entry.thread_key
      ? `thread:${entry.thread_key}`
      : // Lowercased against the invariant locale, never the machine's: this is
        // a grouping key rather than a rendered value, and a Turkish browser
        // folding "I" to "ı" would file one record's conversations differently
        // from every other reader's.
        `subject:${subject.toLowerCase()}`;
    const seen = conversations.get(key);
    const people = peopleOn(entry.links, ctx.nameOf);
    // The list arrives newest-first, so the first row of a conversation is its
    // latest message: that is the date the thread shows it at, and its subject
    // is the one the conversation currently goes by after a mid-thread rename.
    conversations.set(
      key,
      seen
        ? {
            ...seen,
            count: seen.count + 1,
            people: mergePeople(seen.people, people),
          }
        : {
            key,
            at: entry.occurred_at as string,
            subject,
            kind: entry.kind,
            count: 1,
            people,
            direction: entry.direction,
            // Set from the FIRST row of a conversation, which this list orders
            // newest-first — the same row the date and subject above come
            // from, so the stop opens the message it is showing.
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
 * Both conditions come off `email_summary` rather than off the kind string:
 * the server sets it only for `kind=email`, and a message outside the reader's
 * audience carries it with `display_status: withheld`. A stop that offered to
 * open one of those would be a control that draws a placeholder — the reader
 * learns citations do not work, which costs more than the press it saves.
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

// What the exchange WAS, and between whom: "Email to Frédéric de Gombert",
// "Call from Lena Fischer", "Meeting with Lena Fischer and 2 others".
//
// The kind and the direction are two facts a subject line carries neither of,
// and they are the two a reader needs to know whose move it is. A conversation
// with nobody nameable still says what kind it was: that half is always true.
function relationLine(conversation: Exchange, ctx: Ctx): string {
  const what =
    conversation.count > 1
      ? ctx.t("co.spine.exchangeCount", {
          count: formatNumber(conversation.count, ctx.locale),
        })
      : ctx.t(EXCHANGE_KINDS[conversation.kind]);
  const who = withWhom(conversation.people, ctx.t, ctx.locale);
  // A meeting we know the host of names BOTH sides, which is the one exchange
  // where the record can: "Lena Fischer met Frédéric de Gombert". On an account
  // several colleagues work, which of them was in the room is half the answer
  // to what happened.
  //
  // MEETINGS ONLY. `host_user_id` is a column on every activity row, and a mail
  // that carries one was still sent rather than held — reading "met" over an
  // email would rewrite what happened.
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

// Which word joins the exchange to the people on it.
//
// A meeting is WITH everyone in it however it was arranged, so it never takes
// a side. Everything else follows the message: one they sent went TO them, one
// they sent us came FROM them, and an exchange whose direction nothing recorded
// says "with" rather than guessing a side that decides who owes a reply.
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
// recognises and the relation is the half that says whose move it is; run
// together with a separator the subject stopped being findable, reading as one
// long caption where a reader is scanning for a thread they remember.
// `what` is a node rather than a string because a mail conversation names its
// message as a citation that opens, and a call names its subject as text.
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

// A subject with its reply and forward markers taken off, so "Re: Kickoff" and
// "Kickoff" are recognised as one conversation when neither row was threaded
// by capture.
//
// Repeated markers are stripped to the end: a chain that has been replied to
// and forwarded arrives as "Re: Fwd: Kickoff", and taking one prefix off would
// file it apart from the conversation it belongs to. Only the ASCII prefixes
// every mail client writes — a localized "AW:" that slips through costs one
// extra stop, which is the cheap direction to be wrong in.
//
// Returns "" for a subject that is absent, blank, or nothing but markers.
// Those are not conversations the thread can name, and folding them together
// under one empty key would merge unrelated calls into a stop with no title.
function bareSubject(subject: string | null | undefined): string {
  let bare = (subject ?? "").trim();
  let stripped = bare.replace(/^(?:re|fwd?)\s*:\s*/i, "").trim();
  while (stripped !== bare) {
    bare = stripped;
    stripped = bare.replace(/^(?:re|fwd?)\s*:\s*/i, "").trim();
  }
  return bare;
}
