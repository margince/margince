// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The room's settings, opened from the tab's Manage room. They say who may
// still enter and until when, who the buyer turns to, and what a buyer reads
// first. One save runs each change through the server's own verb for it.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { DoorOpen, Lock, PauseCircle } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { FINISHED_STATES, refusalFor } from "../../../components/dealroom";
import { RosterFailedLine, useRoster } from "../../../components/roster";
import { Field, Textarea, TextInput } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { ChoiceList } from "../../../design-system/choicelist";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import {
  DateInput,
  type ISODate,
  isoOrBlank,
} from "../../../design-system/dateinput";
import { Eyebrow } from "../../../design-system/eyebrow";
import { Select } from "../../../design-system/select";
import { useT } from "../../../i18n";
import { problemMessageOf, unwrap } from "../../../lib/problem";
import { navigate } from "../../../lib/router";
import "./dealroomtab.css";

type DealRoom = components["schemas"]["DealRoom"];

/** The three ways a rep sets who may still enter. */
export type RoomAccess = "live" | "paused" | "closed";

type RoomVerb = "pause" | "resume" | "close";

type RoomChanges = Readonly<{
  roomId: string;
  version: number;
  text?: Readonly<{
    title: string;
    welcome_message: string | null;
    steward_user_id?: string;
  }>;
  /** The end of access as an instant, or null to clear it. */
  expiry?: string | null;
  verb?: RoomVerb;
}>;

// The verb that moves a room from one access to another. Closing is final, so
// nothing moves a closed room.
function verbFor(
  from: DealRoom["state"],
  to: RoomAccess,
): RoomVerb | undefined {
  if (from === to) {
    return undefined;
  }
  if (to === "closed") {
    return "close";
  }
  if (from === "live" && to === "paused") {
    return "pause";
  }
  if (from === "paused" && to === "live") {
    return "resume";
  }
  return undefined;
}

function accessOf(state: DealRoom["state"]): RoomAccess | "" {
  return state === "live" || state === "paused" || state === "closed"
    ? state
    : "";
}

// The day a date field shows, and the instant it saves. The instant is the
// last second of that day in UTC, as the room's page reads an end date.
function dayOf(instant: string | null | undefined): ISODate | "" {
  const day = instant ? instant.slice(0, 10) : "";
  return isoOrBlank(day);
}
const endOfDay = (day: string) =>
  day === "" ? null : new Date(`${day}T23:59:59Z`).toISOString();

/**
 * Applies the room's changes. The text goes first, because it returns the
 * version the end date needs. The access verb goes last, because closing
 * ends every other edit.
 */
export function useSaveRoom(dealId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["deal-room-manage"],
    mutationFn: async (changes: RoomChanges) => {
      const path = { path: { id: changes.roomId } };
      let version = changes.version;
      if (changes.text) {
        const updated = unwrap(
          await api.PATCH("/deal-rooms/{id}", {
            params: { ...path, ...ifMatch(version) },
            body: changes.text,
          }),
          t,
        );
        version = requireVersion(updated?.version);
      }
      if (changes.expiry !== undefined) {
        const updated = unwrap(
          await api.PUT("/deal-rooms/{id}/expiry", {
            params: { ...path, ...ifMatch(version) },
            body: { expires_at: changes.expiry },
          }),
          t,
        );
        version = requireVersion(updated?.version);
      }
      if (changes.verb) {
        unwrap(
          await api.POST(`/deal-rooms/{id}/${changes.verb}`, { params: path }),
          t,
        );
      }
    },
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["deal-rooms", dealId] }),
  });
}

/**
 * The dialog. The caller remounts it (a new `key`) on each opening. So the
 * form starts from the room as it stands, not from a cancelled edit.
 */
export function ManageRoomDialog({
  room,
  open,
  onClose,
}: Readonly<{ room: DealRoom; open: boolean; onClose: () => void }>) {
  const t = useT();
  const finished = FINISHED_STATES.has(room.state);
  const refusal = refusalFor(finished, true, t);
  const [access, setAccess] = useState<RoomAccess | "">(accessOf(room.state));
  const [endDay, setEndDay] = useState<ISODate | "">(dayOf(room.expires_at));
  const [steward, setSteward] = useState(room.steward_user_id ?? "");
  const [title, setTitle] = useState(room.title);
  const [welcome, setWelcome] = useState(room.welcome_message ?? "");
  const roster = useRoster("user", open && !finished);
  const save = useSaveRoom(room.deal_id);
  const textChanged =
    title.trim() !== room.title ||
    welcome.trim() !== (room.welcome_message ?? "") ||
    steward !== (room.steward_user_id ?? "");
  const verb = access === "" ? undefined : verbFor(room.state, access);
  const expiryChanged = endDay !== dayOf(room.expires_at);
  const changed = textChanged || expiryChanged || verb !== undefined;
  const accessChoices = [
    {
      value: "live",
      label: t("room.state.live"),
      mark: <DoorOpen aria-hidden />,
      description: t("roomtab.manage.liveHint"),
    },
    {
      value: "paused",
      label: t("room.state.paused"),
      mark: <PauseCircle aria-hidden />,
      description: t("roompage.pauseHint"),
    },
    {
      value: "closed",
      label: t("room.state.closed"),
      mark: <Lock aria-hidden />,
      description: t("roompage.closeHint"),
    },
  ] as const;
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("roompage.manage")}
      intent="form"
      confirmLabel={t("roomtab.manage.save")}
      confirmReason={refusal}
      confirmDisabled={
        refusal === undefined && (!changed || title.trim() === "")
      }
      pending={save.isPending}
      error={save.isError ? problemMessageOf(save.error, t) : null}
      actionsLead={
        <button
          type="button"
          className="link-button"
          onClick={() =>
            navigate({ screen: "deals", id: room.deal_id, id2: "room" })
          }
        >
          {t("roomtab.manage.openPage")}
        </button>
      }
      onConfirm={() =>
        save.mutate(
          {
            roomId: room.id,
            version: requireVersion(room.version),
            text: textChanged
              ? {
                  title: title.trim(),
                  welcome_message:
                    welcome.trim() === "" ? null : welcome.trim(),
                  ...(steward === "" ? {} : { steward_user_id: steward }),
                }
              : undefined,
            expiry: expiryChanged ? endOfDay(endDay) : undefined,
            verb,
          },
          { onSuccess: onClose },
        )
      }
    >
      <div className="roommanage">
        <ChoiceList<RoomAccess>
          legend={t("roompage.accessMenu")}
          layout="cards"
          choices={accessChoices}
          value={access}
          // A finished room opens no more; resuming on an archived deal is
          // the server's to refuse, with its reason.
          disabled={finished}
          onChange={setAccess}
        />
        {verb === "close" ? (
          <Callout tone="warning" title={t("roompage.closeTitle")}>
            {t("roompage.closeBody")}
          </Callout>
        ) : null}
        <div className="roommanage-pair">
          <Field
            label={t("roompage.expiryLabel")}
            hint={t("roomtab.manage.expiryHint")}
          >
            {(control) => (
              <DateInput
                {...control}
                value={endDay}
                disabled={finished}
                onChange={(event) => setEndDay(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("roomtab.manage.contactLabel")}
            hint={t("roomtab.manage.contactHint")}
          >
            {(control) => (
              <Select
                id={control.id}
                value={steward}
                placeholder={t("roomtab.manage.contactOwner")}
                disabled={finished}
                onChange={setSteward}
                options={(roster.data ?? []).map((entry) => ({
                  value: entry.id,
                  label:
                    "display_name" in entry ? entry.display_name : entry.id,
                }))}
              />
            )}
          </Field>
          <RosterFailedLine kind="user" roster={roster} />
        </div>
        <Eyebrow as="h3">{t("roomtab.manage.textHeading")}</Eyebrow>
        <Field label={t("roompage.text.titleLabel")} required>
          {(control) => (
            <TextInput
              {...control}
              value={title}
              disabled={finished}
              onChange={(event) => setTitle(event.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("roompage.text.welcomeLabel")}
          hint={t("roomtab.manage.welcomeHint")}
        >
          {(control) => (
            <Textarea
              {...control}
              rows={3}
              value={welcome}
              disabled={finished}
              onChange={(event) => setWelcome(event.target.value)}
            />
          )}
        </Field>
        <p className="t-caption">{t("roomtab.manage.foot")}</p>
      </div>
    </ConfirmModal>
  );
}
