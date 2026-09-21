// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Deal Room's shared vocabulary: its states, the badge that names one,
// and the read every surface that shows a room starts from — the tab on the
// deal record and the access rail beside it (deal360/dealroomtab.tsx), and
// the room's own page (dealroompage.tsx).

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";

type DealRoomState = components["schemas"]["DealRoomState"];

// The room states in which the room still takes content rather than being a
// record. It mirrors the store's own `publishable` rule; the server refuses
// regardless, so this exists to say WHY before the click rather than to
// enforce anything.
export const FINISHED_STATES: ReadonlySet<DealRoomState> = new Set([
  "closed",
  "expired",
  "archived",
]);

// Each room state's chip label. Keyed by the contract's own closed union rather
// than by string, so a state the contract adds fails the typecheck here — a
// Record<string, …> would compile and render the bare machine word to a rep.
export const STATE_LABELS: Record<DealRoomState, MessageKey> = {
  draft: "room.state.draft",
  building: "room.state.building",
  ready: "room.state.ready",
  publishing: "room.state.publishing",
  live: "room.state.live",
  paused: "room.state.paused",
  closed: "room.state.closed",
  expired: "room.state.expired",
  archived: "room.state.archived",
};

// What each state SAYS, in the badge vocabulary. `live` is the one state that
// is good news about a room — a buyer can walk in right now — so it is the one
// that takes the success tone; the two that stopped a buyer at the door take
// warning; a room nobody can enter again takes danger. The states before a room
// has ever opened are standing rather than verdict and take no tone at all: a
// draft is not going badly.
//
// Keyed by the contract's closed union for the same reason the labels are.
const STATE_TONES: Record<
  DealRoomState,
  "success" | "warning" | "danger" | "accent" | undefined
> = {
  draft: undefined,
  building: undefined,
  ready: undefined,
  publishing: "accent",
  live: "success",
  paused: "warning",
  closed: undefined,
  expired: "warning",
  archived: "danger",
};

// The states whose truth is about THIS MOMENT rather than about something the
// room recorded earlier: a buyer is in the door now, or bytes are moving now.
// They are what earns the badge's breathing dot.
const CURRENT_STATES: ReadonlySet<DealRoomState> = new Set([
  "live",
  "publishing",
]);

/**
 * A room's state as one chip, drawn the same wherever a room is named: the
 * contact's list of the rooms it sits in, and the head of the room's own page.
 *
 * One component rather than a `Badge` at each call site: the tone and the
 * breathing dot are a reading of the state, and a reading spelled twice is
 * how a room comes to look live in one place and merely open in another.
 */
export function RoomStateBadge({ state }: Readonly<{ state: DealRoomState }>) {
  const t = useT();
  return (
    <Badge tone={STATE_TONES[state]} live={CURRENT_STATES.has(state)}>
      {t(STATE_LABELS[state])}
    </Badge>
  );
}

export function useDealRoom(dealId: string) {
  return useQuery({
    queryKey: ["deal-rooms", dealId],
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-rooms", {
        params: { query: { deal_id: dealId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// The sentence a control states instead of accepting a change, or undefined
// when this reader may make it. One function so the row and the form cannot
// disagree about whether a change is possible.
export function refusalFor(
  finished: boolean,
  mayWrite: boolean,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (finished) {
    return t("room.finished");
  }
  if (!mayWrite) {
    return t("room.readOnly");
  }
  return undefined;
}
