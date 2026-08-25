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
} from "../format/format";
import { useLocale, useT } from "../i18n";
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

// The thread, oldest first: the last real conversation, the silence since it,
// then what is dated ahead.
//
// Only three stops, never a full timeline — the history tab is the history.
// This answers "where does this account stand today", and a reader scrolling
// twelve rows to find the gap has lost the one thing the shape is for.
function spineStops(view: Organization360, ctx: Ctx): Stop[] {
  const stops: Stop[] = [];
  const spoke = lastExchange(view, ctx);
  if (spoke) {
    stops.push(spoke);
  }
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
  const since = silenceSince(view);
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
    when: ctx.t("co.spine.days", { count: days }),
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

// The kinds that are an EXCHANGE with the account. The timeline section is
// unfiltered — it carries tasks from the same table — and a task is something
// we wrote to ourselves rather than something that was said.
const EXCHANGE_KINDS: ReadonlySet<string> = new Set([
  "email",
  "call",
  "meeting",
  "note",
  "message",
]);

// The last thing that was actually SAID, and when.
//
// Dated off the exchange itself rather than off `health.last_meeting_at`,
// because those are two different facts and the thread states one of them: a
// meeting date beside an email's subject reads as a meeting about that
// subject. Falls back to the meeting/outbound date only when the timeline is
// withheld or empty — a date with no subject is still the start of the
// thread, and losing it would leave the gap below measuring from nothing.
function lastExchange(view: Organization360, ctx: Ctx): Stop | undefined {
  const said = (view.activities?.data ?? []).find(
    (entry) =>
      EXCHANGE_KINDS.has(entry.kind) &&
      Boolean(entry.subject) &&
      // Already happened, as of the read the rest of the card describes: an
      // `occurred_at DESC` list sorts a meeting booked for next week to the
      // top, and it has not been said yet.
      Boolean(entry.occurred_at) &&
      (entry.occurred_at as string) <= view.as_of,
  );
  const at = said?.occurred_at ?? silenceSince(view);
  if (!at) {
    return undefined;
  }
  return {
    key: "spoke",
    tone: "past",
    when: on(at, ctx),
    title: ctx.t("co.spine.lastSpoke"),
    detail: said?.subject ?? undefined,
  };
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
    return ctx.t("co.spine.unpriced", { count: commercial.open_count });
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
