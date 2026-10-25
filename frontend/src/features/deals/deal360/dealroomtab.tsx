// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal's own Deal Room reading. The record's tab and the room's own page
// (dealroompage.tsx) mount the same sections, so the two cannot disagree.
//
// The tab leads with the room's band, what is still missing, and the brief,
// then sets the documents beside the conversation. Its settings open in a
// dialog; the room's page keeps them for a mailed link.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DoorOpen, ExternalLink, Settings2 } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import {
  FINISHED_STATES,
  refusalFor,
  useDealRoom,
} from "../../../components/dealroom";
import {
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { ErrorLine } from "../../../design-system/errorline";
import { Panel, PanelBody } from "../../../design-system/panel";
import { QueryStates } from "../../../design-system/querystates";
import { useT } from "../../../i18n";
import { useCanWrite } from "../../../lib/capability";
import { problemMessageOf, unwrap } from "../../../lib/problem";
import { buyerLink, DealRoomAccess } from "../dealroomaccess";
import { InviteDialog } from "../dealroomaccess.dialogs";
import { DealRoomConversation } from "../dealroomconversation";
import { RoomBrief } from "./dealroombrief";
import { RoomBand, RoomSetup, StateAction, StateBanner } from "./dealroomdeck";
import { ManageRoomDialog } from "./dealroommanage";
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
  const [inviting, setInviting] = useState(false);
  // The invite dialog sits outside the reading. The setup list can go away and
  // a reload of the room can fail, and the issued link stays either way.
  return (
    <>
      <QueryStates
        query={roomQuery}
        pendingLines={6}
        pendingLabel={t("room.card.title")}
      >
        {room ? (
          <RoomReading
            room={room}
            mayWrite={mayWrite}
            onInvite={() => setInviting(true)}
          />
        ) : roomQuery.isSuccess ? (
          <OpenRoomCard dealId={dealId} dealName={dealName} />
        ) : null}
      </QueryStates>
      {room && mayWrite ? (
        <InviteDialog
          room={room}
          open={inviting}
          onClose={() => setInviting(false)}
        />
      ) : null}
    </>
  );
}

function RoomReading({
  room,
  mayWrite,
  onInvite,
}: Readonly<{ room: DealRoom; mayWrite: boolean; onInvite: () => void }>) {
  const t = useT();
  const finished = FINISHED_STATES.has(room.state);
  const refusal = refusalFor(finished, mayWrite, t);
  const [managing, setManaging] = useState(false);
  // Each opening remounts the dialog, so it starts from the room as it is.
  const [opening, setOpening] = useState(0);
  const manage = () => {
    setOpening((n) => n + 1);
    setManaging(true);
  };
  return (
    <div className="record-stack">
      <RoomBand
        room={room}
        verbs={
          mayWrite ? (
            <>
              <ViewAsBuyerButton room={room} />
              <Button onClick={manage}>
                <Settings2 aria-hidden />
                {t("roompage.manage")}
              </Button>
            </>
          ) : null
        }
      />
      <StateBanner
        room={room}
        action={
          mayWrite ? <StateAction room={room} onManage={manage} /> : undefined
        }
      />
      {mayWrite && !finished ? (
        <RoomSetup room={room} onEditText={manage} onInvite={onInvite} />
      ) : null}
      <RoomBrief roomId={room.id} />
      <DealRoomConversation room={room} refusal={refusal} layout="deck" />
      {/* Who may walk in, and the verbs that change it, ON the tab: the
          record's details pane is shut when a reader arrives. */}
      <DealRoomAccess room={room} mayManage={mayWrite && !finished} />
      {mayWrite ? (
        <ManageRoomDialog
          key={opening}
          room={room}
          open={managing}
          onClose={() => setManaging(false)}
        />
      ) : null}
    </div>
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
      unwrap(
        await api.PATCH("/deal-rooms/{id}", {
          params: { path: { id: room.id }, ...ifMatch(input.version) },
          body: {
            title: input.title,
            welcome_message: input.welcome === "" ? null : input.welcome,
          },
        }),
        t,
      );
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
      return unwrap(
        await api.POST("/deal-rooms", {
          body: { deal_id: dealId, title: roomTitle, source: "manual" },
        }),
        t,
      );
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
        intent="form"
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
      return unwrap(
        await api.POST("/deal-rooms/{id}/preview", {
          params: { path: { id: roomId } },
        }),
        t,
      );
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
