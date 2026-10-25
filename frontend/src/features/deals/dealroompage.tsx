import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import {
  FINISHED_STATES,
  RoomStateBadge,
  refusalFor,
  useDealRoom,
} from "../../components/dealroom";
import {
  Button,
  EmptyState,
  Field,
  OverflowMenu,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { RoomFacts, StateBanner } from "./deal360/dealroomdeck";
import { RoomText, ViewAsBuyerButton } from "./deal360/dealroomtab";
import { DealRoomAccess } from "./dealroomaccess";
import { DealRoomConversation } from "./dealroomconversation";
import "./dealroompage.css";

// The seller's Deal Room page: the room's own identity, its lifecycle verbs
// and the buyer preview, around the same reading its Deal Room tab draws on
// the deal record (deal360/dealroomtab.tsx). Reached from that tab's Manage
// room, from a contact's Deal Rooms panel, or from a mailed link or bookmark.
//
// Everything on this page is live. A document added is shared, a title changed
// is read: the invitation is the only gate, and the seller does not press a
// second button to reach the buyer they already invited.

type DealRoom = components["schemas"]["DealRoom"];

export function DealRoomPage({ dealId }: Readonly<{ dealId: string }>) {
  const t = useT();
  const roomQuery = useDealRoom(dealId);
  const room = roomQuery.data?.data?.[0];
  // The gutter is the PAGE's, so it wears `.wrap` (app/shell.css) OUTSIDE the
  // query's states: a skeleton or a refusal flush against the scroller's edge is
  // the same defect as a loaded room flush against it, and only the outer
  // element is on screen for all three.
  return (
    <div className="wrap">
      <QueryStates
        query={roomQuery}
        pendingLines={6}
        pendingLabel={t("roompage.text.title")}
      >
        {room ? (
          <RoomPage room={room} dealId={dealId} />
        ) : roomQuery.isSuccess ? (
          // No room YET, the one thing an empty state may claim.
          <EmptyState>
            <p>{t("roompage.none")}</p>
          </EmptyState>
        ) : null}
      </QueryStates>
    </div>
  );
}

function RoomPage({
  room,
  dealId,
}: Readonly<{ room: DealRoom; dealId: string }>) {
  const t = useT();
  const mayWrite = useCanWrite("deal_room", "update");
  const finished = FINISHED_STATES.has(room.state);
  const refusal = refusalFor(finished, mayWrite, t);
  return (
    <div className="roompage">
      <header className="roompage-head">
        <div className="roompage-id">
          <p>
            <button
              type="button"
              className="link-button"
              onClick={() => navigate({ screen: "deals", id: dealId })}
            >
              {t("roompage.backToDeal")}
            </button>
          </p>
          <Eyebrow as="span">{t("room.card.title")}</Eyebrow>
          {/* The room's standing reads on the same line as its name, the way a
              record's does: what this is, then how it stands, before anything
              a reader could do to it. It sat among the verbs at the far end of
              the row, where a reader looking for the state found three
              buttons. */}
          <div className="roompage-title-row">
            <Heading size="xlarge" className="t-display">
              {room.title}
            </Heading>
            <RoomStateBadge state={room.state} />
          </div>
          <RoomFacts room={room} />
        </div>
        <div className="roompage-verbs">
          {mayWrite ? <ViewAsBuyerButton room={room} /> : null}
          {mayWrite ? <LifecycleMenu room={room} /> : null}
        </div>
      </header>
      <StateBanner room={room} />
      <RoomText room={room} refusal={refusal} />
      <DealRoomConversation room={room} refusal={refusal} />
      <DealRoomAccess room={room} mayManage={mayWrite} />
    </div>
  );
}

function useRoomVerb(dealId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  return {
    t,
    refresh: () =>
      queryClient.invalidateQueries({ queryKey: ["deal-rooms", dealId] }),
  };
}

function LifecycleMenu({ room }: Readonly<{ room: DealRoom }>) {
  const { t, refresh } = useRoomVerb(room.deal_id);
  const [expiring, setExpiring] = useState(false);
  const [closing, setClosing] = useState(false);
  const move = useMutation({
    mutationFn: async (verb: "pause" | "resume" | "close") => {
      const path = `/deal-rooms/{id}/${verb}` as const;
      unwrap(
        await api.POST(path, {
          params: { path: { id: room.id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      refresh();
      setClosing(false);
    },
  });
  return (
    <>
      <OverflowMenu label={t("roompage.accessMenu")}>
        {room.state === "live" ? (
          <Button
            variant="ghost"
            pending={move.isPending}
            onClick={() => move.mutate("pause")}
          >
            {t("roompage.pause")}
            <span className="roompage-menu-hint">
              {t("roompage.pauseHint")}
            </span>
          </Button>
        ) : null}
        {room.state === "paused" ? (
          <Button
            variant="ghost"
            pending={move.isPending}
            onClick={() => move.mutate("resume")}
          >
            {t("roompage.resume")}
          </Button>
        ) : null}
        {room.state === "live" || room.state === "paused" ? (
          <Button variant="ghost" onClick={() => setClosing(true)}>
            {t("roompage.close")}
            <span className="roompage-menu-hint">
              {t("roompage.closeHint")}
            </span>
          </Button>
        ) : null}
        {!FINISHED_STATES.has(room.state) ? (
          <Button variant="ghost" onClick={() => setExpiring(true)}>
            {t("roompage.setExpiry")}
            <span className="roompage-menu-hint">
              {t("roompage.setExpiryHint")}
            </span>
          </Button>
        ) : null}
      </OverflowMenu>
      <ErrorLine error={move.error} />
      <ConfirmModal
        open={closing}
        onClose={() => setClosing(false)}
        title={t("roompage.closeTitle")}
        confirmLabel={t("roompage.close")}
        confirmVariant="danger"
        pending={move.isPending}
        error={move.isError ? problemMessageOf(move.error, t) : null}
        onConfirm={() => move.mutate("close")}
      >
        <p>{t("roompage.closeBody")}</p>
      </ConfirmModal>
      <ExpiryDialog
        room={room}
        open={expiring}
        onClose={() => setExpiring(false)}
      />
    </>
  );
}

function ExpiryDialog({
  room,
  open,
  onClose,
}: Readonly<{ room: DealRoom; open: boolean; onClose: () => void }>) {
  const { t, refresh } = useRoomVerb(room.deal_id);
  const [date, setDate] = useState(
    room.expires_at ? room.expires_at.slice(0, 10) : "",
  );
  const set = useMutation({
    // The version rides in the variables: a click between a room refresh and
    // the mutation re-arming would otherwise pin a version already gone.
    mutationFn: async (input: { day: string; version: number }) => {
      unwrap(
        await api.PUT("/deal-rooms/{id}/expiry", {
          params: { path: { id: room.id }, ...ifMatch(input.version) },
          body: {
            expires_at:
              input.day === ""
                ? null
                : new Date(`${input.day}T23:59:59Z`).toISOString(),
          },
        }),
        t,
      );
    },
    onSuccess: () => {
      refresh();
      onClose();
    },
  });
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("roompage.setExpiry")}
      intent="form"
      confirmLabel={t("access.save")}
      pending={set.isPending}
      error={set.isError ? problemMessageOf(set.error, t) : null}
      onConfirm={() =>
        set.mutate({ day: date, version: requireVersion(room.version) })
      }
    >
      <Field label={t("roompage.expiryLabel")} hint={t("roompage.expiryHint")}>
        {(control) => (
          <DateInput
            {...control}
            value={isoOrBlank(date)}
            onChange={(e) => setDate(e.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
