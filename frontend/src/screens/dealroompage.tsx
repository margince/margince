import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Pause, Play, Square } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import {
  Button,
  Field,
  OverflowMenu,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import {
  FINISHED_STATES,
  RoomStateBadge,
  refusalFor,
  useDealRoom,
} from "./dealroom";
import { buyerLink, DealRoomAccess, useRoomAttendance } from "./dealroomaccess";
import { DealRoomConversation } from "./dealroomconversation";
import "./dealroompage.css";

// The seller's Deal Room page: one place to decide who may enter, what they
// read, when it goes out, and to hold the conversation. Reached from the deal
// page; the deal page keeps only a card that points here.
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
          <Callout tone="info">{t("roompage.none")}</Callout>
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
          <p className="t-caption">
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
            <h1 className="t-display">{room.title}</h1>
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

// Who is in the room, under its name: how many were invited, how many have
// been through the door, and when a buyer last looked. The counts come from
// the same participants read the Access panel on this page already makes, so
// this is a second READING of one request rather than a second request.
//
// It is a summary and the panel is the register: the line says how many, the
// panel says who — which is why the two are not one fact said twice.
function RoomFacts({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const { invited, active, lastSeen, counted } = useRoomAttendance(room.id);
  if (!counted) {
    return null;
  }
  return (
    <p className="t-caption roompage-facts">
      <span>
        {t("room.card.people", {
          invited: formatNumber(invited, locale),
          active: formatNumber(active, locale),
        })}
      </span>
      {lastSeen ? (
        <span>{t("room.card.lastSeen", { when: lastSeen.slice(0, 10) })}</span>
      ) : null}
    </p>
  );
}

// "View as buyer": a real buyer session, minted for the rep's own hidden
// preview seat and opened through the public screen — so what the rep sees
// is what the buyer gets, release and all. The credential rides in the new
// tab's fragment exactly as a mailed link would, and is never kept here.
function ViewAsBuyerButton({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const preview = useMutation({
    mutationFn: async (roomId: string) => {
      const { data, error } = await api.POST("/deal-rooms/{id}/preview", {
        params: { path: { id: roomId } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (data) => {
      if (data) {
        window.open(buyerLink(data.credential), "_blank", "noopener");
      }
    },
  });
  // Why the preview is not on offer, or undefined when it is.
  //
  // Archived is named first because it is the specific thing a reader can act
  // on: unarchive the room. The general refusal covers the other three
  // conditions the server checks — the caller's grant, being a person rather
  // than an agent, and the deal being writable and live — which a screen cannot
  // tell apart and should not guess between.
  //
  // The button used to gate on archived ALONE, so a colleague who could read
  // the room was offered a preview that failed after the click, with a
  // permission message where the buyer's view should have been.
  //
  // An ABSENT preview_available is an older server that does not answer the
  // question, and offering the button is the honest reading of "unknown": the
  // press still asks, and its refusal is the one this exists to pre-empt.
  const reason = (() => {
    if (room.state === "archived") {
      return t("roompage.previewArchived");
    }
    if (room.preview_available === false) {
      return t("roompage.previewNotYours");
    }
    return undefined;
  })();
  return (
    <>
      <Button
        reason={reason}
        pending={preview.isPending}
        onClick={() => preview.mutate(room.id)}
      >
        <ExternalLink aria-hidden />
        {t("roompage.viewAsBuyer")}
      </Button>
      {preview.isError ? (
        <span className="t-caption t-danger">
          {problemMessageOf(preview.error, t)}
        </span>
      ) : null}
    </>
  );
}

// The state, said in the words a rep needs: what the buyer sees right now,
// and the way back where there is one.
function StateBanner({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  switch (room.state) {
    case "paused":
      return <Callout tone="warn">{t("roompage.banner.paused")}</Callout>;
    case "closed":
      return <Callout tone="info">{t("roompage.banner.closed")}</Callout>;
    case "expired":
      return <Callout tone="warn">{t("roompage.banner.expired")}</Callout>;
    case "archived":
      return <Callout tone="danger">{t("roompage.banner.archived")}</Callout>;
    default:
      return room.expires_at ? (
        <Callout tone="info">
          {t("roompage.banner.liveUntil", {
            when: formatDateAbbrev(room.expires_at, locale, recordZone),
          })}
        </Callout>
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
            small
            variant="ghost"
            pending={move.isPending}
            onClick={() => move.mutate("pause")}
          >
            <Pause aria-hidden />
            {t("roompage.pause")}
            <span className="t-caption roompage-menu-hint">
              {t("roompage.pauseHint")}
            </span>
          </Button>
        ) : null}
        {room.state === "paused" ? (
          <Button
            small
            variant="ghost"
            pending={move.isPending}
            onClick={() => move.mutate("resume")}
          >
            <Play aria-hidden />
            {t("roompage.resume")}
          </Button>
        ) : null}
        {room.state === "live" || room.state === "paused" ? (
          <Button small variant="ghost" onClick={() => setClosing(true)}>
            <Square aria-hidden />
            {t("roompage.close")}
            <span className="t-caption roompage-menu-hint">
              {t("roompage.closeHint")}
            </span>
          </Button>
        ) : null}
        {!FINISHED_STATES.has(room.state) ? (
          <Button small variant="ghost" onClick={() => setExpiring(true)}>
            {t("roompage.setExpiry")}
            <span className="t-caption roompage-menu-hint">
              {t("roompage.setExpiryHint")}
            </span>
          </Button>
        ) : null}
      </OverflowMenu>
      {move.isError ? (
        <p className="t-caption t-danger">{problemMessageOf(move.error, t)}</p>
      ) : null}
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

// Title and welcome text, edited in place. Editorial: reaches the buyer at
// the next publish, and the changes list below says so.
function RoomText({
  room,
  refusal,
}: Readonly<{ room: DealRoom; refusal: string | undefined }>) {
  const { t, refresh } = useRoomVerb(room.deal_id);
  const [title, setTitle] = useState(room.title);
  const [welcome, setWelcome] = useState(room.welcome_message ?? "");
  const save = useMutation({
    mutationFn: async (input: {
      title: string;
      welcome: string;
      version: number;
    }) => {
      const { error } = await api.PATCH("/deal-rooms/{id}", {
        params: { path: { id: room.id }, ...ifMatch(input.version) },
        body: {
          title: input.title,
          welcome_message: input.welcome === "" ? null : input.welcome,
        },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
    onSuccess: refresh,
  });
  const dirty =
    title !== room.title || welcome !== (room.welcome_message ?? "");
  return (
    <Panel title={t("roompage.text.title")} sub={t("roompage.text.sub")}>
      <PanelBody>
        <div className="form-stack">
          <Field label={t("roompage.text.titleLabel")}>
            {(control) => (
              <TextInput
                {...control}
                value={title}
                disabled={refusal !== undefined}
                onChange={(e) => setTitle(e.target.value)}
              />
            )}
          </Field>
          <Field label={t("roompage.text.welcomeLabel")}>
            {(control) => (
              <Textarea
                {...control}
                rows={3}
                value={welcome}
                disabled={refusal !== undefined}
                onChange={(e) => setWelcome(e.target.value)}
              />
            )}
          </Field>
          {refusal ? (
            <p className="t-caption">{refusal}</p>
          ) : (
            <div className="card-actions">
              <Button
                small
                disabled={!dirty || title.trim() === ""}
                pending={save.isPending}
                onClick={() =>
                  save.mutate({
                    title: title.trim(),
                    welcome: welcome.trim(),
                    version: requireVersion(room.version),
                  })
                }
              >
                {t("access.save")}
              </Button>
              {save.isError ? (
                <span className="t-caption t-danger">
                  {problemMessageOf(save.error, t)}
                </span>
              ) : null}
            </div>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}
