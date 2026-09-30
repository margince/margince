import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import {
  Button,
  EmptyState,
  Field,
  OverflowMenu,
  TextInput,
} from "../design-system/atoms";
import { Callout, type CalloutTone } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Eyebrow } from "../design-system/eyebrow";
import { Heading } from "../design-system/heading";
import { formatDateAbbrev } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { RoomFacts, RoomText, ViewAsBuyerButton } from "./deal360/dealroomtab";
import {
  FINISHED_STATES,
  RoomStateBadge,
  refusalFor,
  useDealRoom,
} from "./dealroom";
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
      <div className="roompage-grid">
        <div className="roompage-main">
          <RoomText room={room} refusal={refusal} />
          <DealRoomConversation room={room} refusal={refusal} />
        </div>
        <div className="roompage-side">
          <DealRoomAccess room={room} mayManage={mayWrite} />
        </div>
      </div>
    </div>
  );
}

/**
 * One band for every standing state of a room: five that differed only in tone
 * and wording had each drawn their own. `standing`, and so silent — the state
 * is true as the page renders — and the claim IS the notice, with no body.
 */
function StateNotice({
  tone,
  claim,
}: Readonly<{ tone?: CalloutTone; claim: string }>) {
  return <Callout tone={tone} kind="standing" title={claim} />;
}

// The state, said in the words a rep needs: what the buyer sees right now,
// and the way back where there is one.
function StateBanner({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  switch (room.state) {
    case "paused":
      return <StateNotice tone="warning" claim={t("roompage.banner.paused")} />;
    case "closed":
      return <StateNotice claim={t("roompage.banner.closed")} />;
    case "expired":
      return (
        <StateNotice tone="warning" claim={t("roompage.banner.expired")} />
      );
    case "archived":
      return (
        <StateNotice tone="danger" claim={t("roompage.banner.archived")} />
      );
    default:
      return room.expires_at ? (
        <StateNotice
          claim={t("roompage.banner.liveUntil", {
            when: formatDateAbbrev(room.expires_at, locale, recordZone),
          })}
        />
      ) : null;
  }
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
      const { error } = await api.POST(path, {
        params: { path: { id: room.id } },
      });
      if (error) {
        throwProblem(error, t);
      }
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
      const { error } = await api.PUT("/deal-rooms/{id}/expiry", {
        params: { path: { id: room.id }, ...ifMatch(input.version) },
        body: {
          expires_at:
            input.day === ""
              ? null
              : new Date(`${input.day}T23:59:59Z`).toISOString(),
        },
      });
      if (error) {
        throwProblem(error, t);
      }
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
      confirmLabel={t("access.save")}
      pending={set.isPending}
      error={set.isError ? problemMessageOf(set.error, t) : null}
      onConfirm={() =>
        set.mutate({ day: date, version: requireVersion(room.version) })
      }
    >
      <Field label={t("roompage.expiryLabel")} hint={t("roompage.expiryHint")}>
        {(control) => (
          <TextInput
            {...control}
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
