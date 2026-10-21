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

import { type ReactNode, useRef } from "react";
import { PanelBody } from "../../../design-system/panel";
import { useScrollRegion } from "../../../design-system/scrollregion";
import { useLocale, useT } from "../../../i18n";
import { useRecordZone } from "../../../lib/recordzone";
import "./spine.css";
import { spineStops, todayColumn } from "./spine.stops";

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
  // The newest contact the server found, over every page of the timeline.
  // Only the account read sends it.
  last_contact?: { at: string } | null;
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
      // What the message is filed against. The contacts among them are who was
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
      // Meeting only: whether it was held, booked or called off.
      meeting_status?: string | null;
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
export type Stop = {
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
  // conversation the thread cannot name a contact for says the kind alone
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
  const axis = useRef<HTMLDivElement>(null);
  const axisRegion = useScrollRegion(axis, t("record.timeline"));
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
  const columns = stops.map(todayColumn).join(" ");
  return (
    <PanelBody className="co-spine">
      {/* The axis runs ACROSS, not down: a reader takes the account's shape —
          what happened, how long nothing did, what is dated next — in one
          left-to-right glance, which is the reading a stacked list cannot
          give. It scrolls rather than wraps, because a time axis that wrapped
          onto a second line would put a later stop to the left of an earlier
          one. */}
      <div ref={axis} className="co-spine-scroll" {...axisRegion}>
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
