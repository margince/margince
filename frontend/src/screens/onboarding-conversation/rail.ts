// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { ConversationState } from "./conversation-types";

// Where the setup journey is, as five stops. Derived from the machine rather
// than tracked beside it, so the rail cannot disagree with the conversation.
//
// The stops are NOT the phases: READ is already finished by the time the
// two-column view first renders, CONFIRM covers the whole clarify/review/
// manual cluster, and the invite is the doorway to VOICE rather than a stop
// of its own. A member never reaches voice, so their rail has four stops —
// a greyed step that will never happen is a promise the flow does not keep.

export type RailStop = Readonly<{
  key: "read" | "confirm" | "voice" | "connect" | "prefs";
  labelKey: MessageKey;
}>;

export type RailStopState = "done" | "now" | "todo";

const CREATOR_STOPS: readonly RailStop[] = [
  { key: "read", labelKey: "ob.rail.read" },
  { key: "confirm", labelKey: "ob.rail.confirm" },
  { key: "voice", labelKey: "ob.rail.voice" },
  { key: "connect", labelKey: "ob.rail.connect" },
  { key: "prefs", labelKey: "ob.rail.prefs" },
];

// The creator's rail minus the stop the member path never visits.
const MEMBER_STOPS: readonly RailStop[] = CREATOR_STOPS.filter(
  (stop) => stop.key !== "voice",
);

export function railStops(memberPath: boolean): readonly RailStop[] {
  return memberPath ? MEMBER_STOPS : CREATOR_STOPS;
}

// The stop the conversation is standing on. `null` while the company act is
// still reading, because the read happens on the gate rather than in the rail's
// surface and marking CONFIRM as current there would point at an empty panel.
export function currentStop(state: ConversationState): RailStop["key"] | null {
  switch (state.act) {
    case "welcome":
      return null;
    case "company":
      return state.phase === "co.intro" || state.phase === "co.reading"
        ? null
        : "confirm";
    // The invite asks whether the voice stop happens at all, so it stands on
    // that stop: a rail pointing at the stop the question is about.
    case "invite":
    case "voice":
      return "voice";
    // The team act is a creator's way OUT of the personal stops, not one of
    // them: no stop is current, and none is promised.
    case "team":
      return null;
    // Every account the setup asks for — mailbox and LinkedIn alike — belongs to
    // this one stop. A stop per integration would grow the rail once per provider
    // for something the reader already reads as "connecting".
    case "connect":
      return "connect";
    case "prefs":
    case "done":
      return "prefs";
  }
}

// The read is its own stop and its own truth: it is done the moment the server
// says so, whether or not the conversation has moved on.
function readStopState(
  state: ConversationState,
  pastTheRead: boolean,
): RailStopState {
  if (state.readCompleted || pastTheRead) {
    return "done";
  }
  return state.phase === "co.reading" ? "now" : "todo";
}

// Where a stop sits relative to the one the conversation is standing on.
function stopStateByPosition(
  stop: RailStop["key"],
  state: ConversationState,
  index: number,
  currentIndex: number,
): RailStopState {
  if (currentIndex < 0 || index < 0 || index > currentIndex) {
    return "todo";
  }
  if (index < currentIndex) {
    return "done";
  }
  // The last stop only reads `done` once the flow actually finished, so
  // it does not claim completion while the user is still choosing.
  return stop === "prefs" && state.act === "done" ? "done" : "now";
}

export function stopState(
  stop: RailStop["key"],
  state: ConversationState,
): RailStopState {
  const stops = railStops(state.memberPath).map((entry) => entry.key);
  const currentIndex = currentIndexOf(currentStop(state), stops);

  // The team act stands on no stop, so `currentIndex` is -1 and the position
  // arithmetic would read every stop as `todo` — including the two the creator
  // finished to get here, and including `read` on a restored flow where
  // `readCompleted` has not come back yet. It is a way OUT of the personal
  // stops: those stay `todo` because this creator will not be walking them.
  if (state.act === "team") {
    return stop === "read" || stop === "confirm" ? "done" : "todo";
  }
  if (stop === "read") {
    return readStopState(state, currentIndex > 0);
  }
  return stopStateByPosition(stop, state, stops.indexOf(stop), currentIndex);
}

function currentIndexOf(
  current: RailStop["key"] | null,
  stops: readonly RailStop["key"][],
) {
  return current === null ? -1 : stops.indexOf(current);
}

// The clarify surface (a legal-entity pick today) takes the whole screen
// from inside the CONFIRM stop, but not every run reaches it — most reads
// resolve the entity on their own. It is a detour off the fixed sequence,
// not a numbered stop in it, so anything naming "step N of M" while this is
// live is claiming a slot the flow does not always have. This is the one
// place that knows the difference; callers ask rather than re-deriving it
// from phase and pendingQuestion themselves.
export function isDetour(state: ConversationState): boolean {
  return state.phase === "co.clarify" && state.pendingQuestion !== null;
}
