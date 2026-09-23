// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's own Deal Room reading, shared by two doors: the record's Deal
// Room tab (deals.tsx) and the room's own page (dealroompage.tsx), reached
// from a mailed link or a bookmark. Both mount the same sections rather than
// each drawing its own — a second copy of "who is in the room" is how a tab
// and a page come to disagree about it.
//
// The tab is the narrower of the two: it carries the reading and the editable
// text, not the room's own verbs (pause, close, set an expiry) or the "view
// as buyer" preview, which stay on the room's own page where the room's
// identity band already stands.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DoorOpen, ExternalLink } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { useCanWrite } from "../../app/capability";

import {
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "../common";
import { FINISHED_STATES, refusalFor, useDealRoom } from "../dealroom";
import {
  buyerLink,
  DealRoomAccess,
  useRoomAttendance,
} from "../dealroomaccess";
import { DealRoomConversation } from "../dealroomconversation";
import "./dealroomtab.css";

type DealRoom = components["schemas"]["DealRoom"];

/**
 * DealRoomTab is the record's own door into the room: the same reading and
 * editable text the room's own page draws, without the page's identity band
 * or its verbs. A deal with no room yet gets the one control that opens one.
 */
export function DealRoomTab({
  dealId,
  dealName,
}: Readonly<{ dealId: string; dealName: string }>) {
  const t = useT();
  const roomQuery = useDealRoom(dealId);
  const room = roomQuery.data?.data?.[0];
  const mayWrite = useCanWrite("deal_room", "update");
  return (
    <QueryStates
      query={roomQuery}
      pendingLines={6}
      pendingLabel={t("room.card.title")}
    >
      {room ? (
        <RoomReading room={room} mayWrite={mayWrite} />
      ) : roomQuery.isSuccess ? (
        <OpenRoomCard dealId={dealId} dealName={dealName} />
      ) : null}
    </QueryStates>
  );
}

function RoomReading({
  room,
  mayWrite,
}: Readonly<{ room: DealRoom; mayWrite: boolean }>) {
  const t = useT();
  const finished = FINISHED_STATES.has(room.state);
  const refusal = refusalFor(finished, mayWrite, t);
  return (
    <div className="record-stack">
      {/* Who has been in, and the one way to see what they see. The preview
          stands with the attendance rather than inside a panel: it is the
          room's own verb, not a verb about its access list or its text. */}
      <div className="roomtab-head">
        <RoomFacts room={room} />
        {mayWrite ? <ViewAsBuyerButton room={room} /> : null}
      </div>
      {/* Who may walk in, and the verbs that change it, ON the tab rather than
          only in the record's details pane: the pane is shut when a reader
          arrives, so a room they had just opened offered them no way to let
          anybody into it. */}
      <DealRoomAccess room={room} mayManage={mayWrite && !finished} />
      <RoomText room={room} refusal={refusal} />
      <DealRoomConversation room={room} refusal={refusal} />
    </div>
  );
}

// Who is in the room, under its name: how many were invited, how many have
// been through the door, and when a buyer last looked. The counts come from
// the same participants read the Access panel makes, so this is a second
// READING of one request rather than a second request.
//
// It is a summary and the panel is the register: the line says how many, the
// panel says who — which is why the two are not one fact said twice.
export function RoomFacts({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const { invited, active, lastSeen, counted } = useRoomAttendance(room.id);
  if (!counted) {
    return null;
  }
  return (
    <p className="t-caption roompage-facts">
      <span>
        {t("room.card.contacts", {
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

// Title and welcome text, edited in place. Editorial: reaches the buyer at
// the next publish, and the changes list below says so.
export function RoomText({
  room,
  refusal,
}: Readonly<{ room: DealRoom; refusal: string | undefined }>) {
  const t = useT();
  const queryClient = useQueryClient();
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
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["deal-rooms", room.deal_id] }),
  });
  const dirty =
    title !== room.title || welcome !== (room.welcome_message ?? "");
  return (
    <Panel title={t("roompage.text.title")}>
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
            <p>{refusal}</p>
          ) : (
            <div className="card-actions">
              <Button
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
              <ErrorLine inline error={save.error} />
            </div>
          )}
        </div>
      </PanelBody>
    </Panel>
  );
}

// A deal with no room yet: the one control that opens one, and nothing else —
// the room's own page draws everything a rep does once it exists.
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
        body: { deal_id: dealId, title: roomTitle, source: "manual" },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    // No navigation: the tab is already where a rep landed, and invalidating
    // the room read is what turns this same tab into the reading it just
    // opened, rather than sending the reader on to a page they did not ask
    // for.
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["deal-rooms", dealId] });
      setCreating(false);
    },
  });
  if (!mayCreate) {
    return null;
  }
  return (
    <Panel title={t("room.card.title")}>
      <PanelBody>
        {/* The tab a reader opened on a deal with no room is a state, not a
            missing card: it says what a room is for and carries the one verb
            that opens one, the way every other empty surface on this product
            invites the thing it lacks. */}
        <EmptyState
          title={t("room.create.open")}
          action={
            <Button onClick={() => setCreating(true)}>
              <DoorOpen aria-hidden />
              {t("room.create.open")}
            </Button>
          }
        >
          {t("room.create.sub")}
        </EmptyState>
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

// "View as buyer": a real buyer session, minted for the rep's own hidden
// preview seat and opened through the public screen — so what the rep sees
// is what the buyer gets, release and all. The credential rides in the new
// tab's fragment exactly as a mailed link would, and is never kept here.
export function ViewAsBuyerButton({ room }: Readonly<{ room: DealRoom }>) {
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
  // conditions the server checks — the caller's grant, being a contact rather
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
      <ErrorLine inline error={preview.error} />
    </>
  );
}
