// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account as ONE thread, in time order: what has happened, the silence
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

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { PanelBody } from "../design-system/panel";
import {
  calendarDaysBetween,
  formatDateAbbrev,
  formatMoneyCompact,
  formatNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./companyspine.css";

type Organization360 = components["schemas"]["Organization360"];

// One stop on the thread. `tone` is what the dot and the rule say about it,
// never decoration: `past` is filed, `gap` is the waiting itself, `ahead` is
// dated and has not happened.
type Stop = {
  key: string;
  tone: "past" | "gap" | "ahead" | "overdue";
  when: string;
  title: ReactNode;
  detail?: ReactNode;
};

/**
 * CompanySpine is the account's story as a vertical thread.
 *
 * It reads the same payload the cards below it read and adds nothing — every
 * stop is a record, a date, or the arithmetic between two dates. What it adds
 * is ORDER: the reader sees the last real contact, how long ago that was, and
 * what is dated ahead, in one downward glance.
 */
export function CompanySpine({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  if (!view) {
    return null;
  }
  const stops = spineStops(view, { t, locale, zone });
  if (stops.length === 0) {
    return null;
  }
  return (
    <PanelBody className="co-spine">
      <ol className="co-spine-track">
        {stops.map((stop) => (
          <li key={stop.key} className={`co-spine-stop co-spine-${stop.tone}`}>
            <span className="co-spine-when">{stop.when}</span>
            <span className="co-spine-dot" aria-hidden="true" />
            <span className="co-spine-what">
              <span className="co-spine-title">{stop.title}</span>
              {stop.detail && (
                <span className="co-spine-detail">{stop.detail}</span>
              )}
            </span>
          </li>
        ))}
      </ol>
    </PanelBody>
  );
}

type Ctx = {
  t: ReturnType<typeof useT>;
  locale: ReturnType<typeof useLocale>["locale"];
  zone: string;
};

// The thread, oldest first: the conversations that have happened, the silence
// since the last of them, then what is dated ahead.
//
// One stop per CONVERSATION, not per message and not per row of the timeline.
// A reader arriving at an account they do not remember asks "what was going on
// here", and a single stop naming the newest email answered that with one
// line out of a story — the June kickoff that explains the account was on the
// page, three scrolls down, under the day's suggestions. Grouping is what
// keeps this a thread rather than the history tab: six emails in two
// conversations are two stops, and the tab is still where an individual
// message is read.
function spineStops(view: Organization360, ctx: Ctx): Stop[] {
  const stops: Stop[] = [...pastStops(view, ctx)];
  const waiting = silence(view, ctx);
  if (waiting) {
    stops.push(waiting);
  }
  for (const ahead of aheadStops(view, ctx)) {
    stops.push(ahead);
  }
  return stops;
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
function silenceSince(view: Organization360): string | undefined {
  return view.last_outbound_at ?? view.health?.last_meeting_at ?? undefined;
}

// Whether they have answered the last thing we sent.
//
// Compared against our own last outbound rather than against whatever
// `silenceSince` fell back to: a reply that came BEFORE our latest message
// does not answer it, and an account whose last word is ours is waiting on
// them however long ago they last wrote.
function silence(view: Organization360, ctx: Ctx): Stop | undefined {
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
    when: ctx.t("co.spine.days", { count: formatNumber(days, ctx.locale) }),
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
function aheadStops(view: Organization360, ctx: Ctx): Stop[] {
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
  const close = view.state_strip?.commercial?.next_close_on;
  if (close) {
    stops.push({
      key: "close",
      tone: "ahead",
      when: on(close, ctx),
      title: ctx.t("co.spine.expectedClose"),
      detail: pipelineValue(view, ctx),
    });
  }
  return stops;
}

function on(at: string, ctx: Ctx): string {
  return formatDateAbbrev(at, ctx.locale, ctx.zone);
}

// The kinds that are an EXCHANGE with the account, each with what a single one
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
// Three is what fits above the fold beside the gap and what is left ahead. An
// account with more history than that has its older conversations on the
// history tab, and the reader is told the count rather than left to assume
// three is all there was.
const PAST_STOPS = 3;

// One conversation, as the thread tells it.
type Exchange = {
  readonly at: string;
  readonly subject: string;
  readonly kind: ExchangeKind;
  readonly count: number;
};

// What was actually SAID here, newest conversation last.
//
// Dated off the exchanges themselves rather than off `health.last_meeting_at`,
// because those are two different facts and the thread states one of them: a
// meeting date beside an email's subject reads as a meeting about that
// subject. Falls back to the meeting/outbound date only when the timeline is
// withheld or empty — a date with no subject is still the start of the thread,
// and losing it would leave the gap below measuring from nothing.
function pastStops(view: Organization360, ctx: Ctx): Stop[] {
  const conversations = exchanges(view);
  if (conversations.length === 0) {
    const at = silenceSince(view);
    return at
      ? [
          {
            key: "spoke",
            tone: "past",
            when: on(at, ctx),
            title: ctx.t("co.spine.lastSpoke"),
          },
        ]
      : [];
  }
  // Newest conversations, then re-read oldest-first: the thread runs down the
  // page in time order, and it is the OLDEST that gets dropped when an account
  // has more history than the thread draws.
  const shown = conversations.slice(0, PAST_STOPS).reverse();
  const earlier = conversations.length - shown.length;
  const stops = shown.map((conversation, index) =>
    pastStop(conversation, index === shown.length - 1, ctx),
  );
  if (earlier > 0) {
    stops.unshift({
      key: "earlier",
      tone: "past",
      when: on(conversations[conversations.length - 1].at, ctx),
      title:
        earlier === 1
          ? ctx.t("co.spine.earlierOne")
          : ctx.t("co.spine.earlier", {
              count: formatNumber(earlier, ctx.locale),
            }),
    });
  }
  return stops;
}

// A conversation's own stop. The newest one keeps "You last spoke" as its
// title, because the gap below is counted from it and a reader has to see what
// the waiting is waiting on. The older ones lead with their subject: they are
// what happened, and repeating one label down the thread would say nothing.
function pastStop(conversation: Exchange, newest: boolean, ctx: Ctx): Stop {
  const messages =
    conversation.count > 1
      ? ctx.t("co.spine.exchangeCount", {
          count: formatNumber(conversation.count, ctx.locale),
        })
      : ctx.t(EXCHANGE_KINDS[conversation.kind]);
  return {
    key: `said-${conversation.at}-${conversation.subject}`,
    tone: "past",
    when: on(conversation.at, ctx),
    title: newest ? ctx.t("co.spine.lastSpoke") : conversation.subject,
    detail: newest ? conversation.subject : messages,
  };
}

// The account's conversations, newest first, each folded to one entry.
//
// `thread_key` is capture's own conversation id and is what makes a thread a
// thread; a note or a hand-logged call carries none, so those fall back to
// their subject. Grouping by subject alone would merge two unrelated "Re:
// Update" exchanges — acceptable as a fallback for the rows that have no
// better key, wrong as the rule.
function exchanges(view: Organization360): Exchange[] {
  const conversations = new Map<string, Exchange>();
  for (const entry of view.activities?.data ?? []) {
    const subject = entry.subject;
    if (
      !isExchange(entry.kind) ||
      !subject ||
      // Already happened, as of the read the rest of the card describes: an
      // `occurred_at DESC` list sorts a meeting booked for next week to the
      // top, and it has not been said yet.
      !entry.occurred_at ||
      entry.occurred_at > view.as_of
    ) {
      continue;
    }
    const key = entry.thread_key ?? `subject:${bareSubject(subject)}`;
    const seen = conversations.get(key);
    // The list arrives newest-first, so the first row of a conversation is its
    // latest message: that is the date the thread shows it at, and its subject
    // is the one the conversation currently goes by after a mid-thread rename.
    conversations.set(
      key,
      seen
        ? { ...seen, count: seen.count + 1 }
        : {
            at: entry.occurred_at,
            subject: bareSubject(subject),
            kind: entry.kind,
            count: 1,
          },
    );
  }
  return [...conversations.values()];
}

// A subject with its reply and forward markers taken off, so "Re: Kickoff" and
// "Kickoff" are recognised as one conversation when neither row was threaded
// by capture. Only the two ASCII prefixes every mail client writes — a
// localized "AW:" that slips through costs one extra stop, while a greedy
// pattern would merge conversations that merely start alike.
function bareSubject(subject: string): string {
  return subject.replace(/^(?:re|fwd?)\s*:\s*/i, "").trim() || subject.trim();
}

// What is riding on the close date. An unpriced pipeline says so rather than
// printing a zero — an account whose deals nobody has costed is not an
// account worth nothing.
function pipelineValue(view: Organization360, ctx: Ctx): ReactNode {
  const commercial = view.state_strip?.commercial;
  if (!commercial) {
    return undefined;
  }
  if (
    commercial.open_pipeline_minor_base == null ||
    !commercial.base_currency
  ) {
    return ctx.t("co.spine.unpriced", {
      count: formatNumber(commercial.open_count, ctx.locale),
    });
  }
  // formatMoneyCompact, not a formatter of this file's own: the app has one
  // locale mapping and one minor-unit table, and a second Intl call here
  // would round a currency the shared one does not.
  return ctx.t("co.spine.worth", {
    amount: formatMoneyCompact(
      commercial.open_pipeline_minor_base,
      commercial.base_currency,
      ctx.locale,
    ),
  });
}
