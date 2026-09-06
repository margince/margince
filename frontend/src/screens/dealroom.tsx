// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal page's Deal Room card, and the door to the room: one card saying
// the room's state, who is in it, whether the buyer has seen the latest
// changes and what came back from their side — with the page one click away.
// A deal without a room gets the one control that opens one.
//
// The room itself lives on its own page (dealroompage.tsx). The card never
// edits anything: a rep who wants to change the room goes where the whole of
// it is visible, rather than half-editing it from the deal's margin.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DoorOpen } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Badge, Button, Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { useRoomAttendance } from "./dealroomaccess";

type DealRoom = components["schemas"]["DealRoom"];
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
// warn; a room nobody can enter again takes danger. The states before a room
// has ever opened are standing rather than verdict and take no tone at all: a
// draft is not going badly.
//
// Keyed by the contract's closed union for the same reason the labels are.
const STATE_TONES: Record<
  DealRoomState,
  "success" | "warn" | "danger" | "accent" | undefined
> = {
  draft: undefined,
  building: undefined,
  ready: undefined,
  publishing: "accent",
  live: "success",
  paused: "warn",
  closed: undefined,
  expired: "warn",
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
 * A room's state as one chip, drawn the same on the deal's card and at the head
 * of the room's own page.
 *
 * One component rather than a `Badge` at each of the two call sites: the tone
 * and the breathing dot are a reading of the state, and a reading spelled twice
 * is how a room comes to look live in the margin of the deal and merely open on
 * its own page.
 */
export function RoomStateBadge({ state }: Readonly<{ state: DealRoomState }>) {
  const t = useT();
  return (
    <Badge tone={STATE_TONES[state]} live={CURRENT_STATES.has(state)}>
      {t(STATE_LABELS[state])}
    </Badge>
  );
}

/**
 * The deal page's Deal Room card. A deal with no room gets the one control
 * that opens one; a deal with a room gets its summary and the way in.
 */
export function DealRoomAside({
  dealId,
  dealName,
}: Readonly<{ dealId: string; dealName: string }>) {
  const t = useT();
  const roomQuery = useDealRoom(dealId);
  const room = roomQuery.data?.data?.[0];
  return (
    <QueryStates
      query={roomQuery}
      pendingLines={3}
      pendingLabel={t("room.card.title")}
    >
      {room ? (
        <RoomCard room={room} />
      ) : roomQuery.isSuccess ? (
        <OpenRoomCard dealId={dealId} dealName={dealName} />
      ) : null}
    </QueryStates>
  );
}

function RoomCard({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const { invited, active, lastSeen } = useRoomAttendance(room.id);
  return (
    <Panel
      title={t("room.card.title")}
      titleAction={<RoomStateBadge state={room.state} />}
    >
      <PanelBody>
        {/* The room's own name is what this card is ABOUT — the panel's title
            is the product's word for the surface — so it takes the card-title
            rung rather than reading as the first line of the caption under it. */}
        <p className="t-h3">{room.title}</p>
        <p className="t-caption">
          {t("room.card.people", {
            invited: formatNumber(invited, locale),
            active: formatNumber(active, locale),
          })}
        </p>
        {lastSeen ? (
          <p className="t-caption">
            {t("room.card.lastSeen", { when: lastSeen.slice(0, 10) })}
          </p>
        ) : null}
        <div className="card-actions">
          <Button
            small
            onClick={() =>
              navigate({ screen: "deals", id: room.deal_id, id2: "room" })
            }
          >
            <DoorOpen aria-hidden />
            {t("room.card.open")}
          </Button>
        </div>
      </PanelBody>
    </Panel>
  );
}

function OpenRoomCard({
  dealId,
  dealName,
}: Readonly<{ dealId: string; dealName: string }>) {
  const t = useT();
  const mayCreate = useCanWrite("deal_room", "create");
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState(
    t("room.create.defaultTitle", { deal: dealName }),
  );
  const create = useMutation({
    mutationFn: async (roomTitle: string) => {
      const { data, error } = await api.POST("/deal-rooms", {
        body: { deal_id: dealId, title: roomTitle, source: "ui" },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["deal-rooms", dealId] });
      setCreating(false);
      navigate({ screen: "deals", id: dealId, id2: "room" });
    },
  });
  if (!mayCreate) {
    return null;
  }
  return (
    <Panel title={t("room.card.title")} sub={t("room.create.sub")}>
      <PanelBody>
        <div className="card-actions">
          <Button small onClick={() => setCreating(true)}>
            <DoorOpen aria-hidden />
            {t("room.create.open")}
          </Button>
        </div>
      </PanelBody>
      <ConfirmModal
        open={creating}
        onClose={() => setCreating(false)}
        title={t("room.create.open")}
        confirmLabel={t("room.create.confirm")}
        confirmDisabled={title.trim() === ""}
        pending={create.isPending}
        error={create.isError ? problemMessageOf(create.error, t) : null}
        onConfirm={() => create.mutate(title.trim())}
      >
        <Field
          label={t("room.create.titleLabel")}
          hint={t("room.create.titleHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </Panel>
  );
}

export function useDealRoom(dealId: string, enabled = true) {
  return useQuery({
    // Off in overlay mode, where the deal is a mirror from another system of
    // record and its sub-resources answer 422.
    enabled,
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
