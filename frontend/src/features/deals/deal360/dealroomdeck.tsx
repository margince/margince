// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The parts of the room's reading above its documents. They are the band, a
// stopped room's notice, what the room still lacks, and the attendance line.
// The tab draws all four; the room's own page draws the notice and the line.

import { DoorOpen } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../../../api/schema";
import { requireVersion } from "../../../api/version";
import { RoomStateBadge } from "../../../components/dealroom";
import { Button } from "../../../design-system/atoms";
import { AvatarStack } from "../../../design-system/avatarstack";
import { Callout, type CalloutTone } from "../../../design-system/callout";
import { Checklist, ChecklistStep } from "../../../design-system/checklist";
import { ErrorLine } from "../../../design-system/errorline";
import { Eyebrow } from "../../../design-system/eyebrow";
import { Heading } from "../../../design-system/heading";
import { Panel, PanelBody } from "../../../design-system/panel";
import { formatDateAbbrev, formatNumber } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useRecordZone } from "../../../lib/recordzone";
import { useParticipants, useRoomAttendance } from "../dealroomaccess";
import { useRoomThreads } from "../dealroomconversation";
import { useRoomDocuments } from "../dealroomdocuments";
import { useSaveRoom } from "./dealroommanage";
import "./dealroomtab.css";

type DealRoom = components["schemas"]["DealRoom"];

// The room's band: what it is, how it stands, what a buyer reads first and
// who is in it. The verbs that act on the whole room stand at its end.
export function RoomBand({
  room,
  verbs,
}: Readonly<{ room: DealRoom; verbs: ReactNode }>) {
  const t = useT();
  const headingId = useId();
  const seats = (useParticipants(room.id).data?.data ?? []).filter(
    (p) => !p.revoked_at,
  );
  return (
    <section className="roomtab-band" aria-labelledby={headingId}>
      <div className="roomtab-band-id">
        <span className="roomtab-band-kind">
          <DoorOpen aria-hidden />
          <Eyebrow>{t("room.card.title")}</Eyebrow>
          <RoomStateBadge state={room.state} />
        </span>
        <Heading size="large" id={headingId} className="roomtab-band-title">
          {room.title}
        </Heading>
        {room.welcome_message ? (
          <p className="roomtab-band-welcome">{room.welcome_message}</p>
        ) : (
          <p className="roomtab-band-welcome t-caption">
            {t("roomtab.welcomeEmpty")}
          </p>
        )}
        <div className="roomtab-band-seats">
          {seats.length > 0 ? (
            <AvatarStack
              max={4}
              contacts={seats.map((p) => ({
                name: p.full_name,
                identity: p.id,
              }))}
            />
          ) : null}
          <RoomFacts room={room} />
        </div>
      </div>
      {verbs ? <div className="roomtab-verbs">{verbs}</div> : null}
    </section>
  );
}

// The way back from a stopped room, beside the sentence that says it stopped:
// a paused room resumes in place; anything else is the dialog's.
export function StateAction({
  room,
  onManage,
}: Readonly<{ room: DealRoom; onManage: () => void }>) {
  const t = useT();
  const save = useSaveRoom(room.deal_id);
  if (room.state === "paused") {
    return (
      <>
        <Button
          pending={save.isPending}
          onClick={() =>
            save.mutate({
              roomId: room.id,
              version: requireVersion(room.version),
              verb: "resume",
            })
          }
        >
          {t("roompage.resume")}
        </Button>
        <ErrorLine inline error={save.error} />
      </>
    );
  }
  return <Button onClick={onManage}>{t("roompage.manage")}</Button>;
}

/**
 * One band for every standing state of a room, said in the words a rep needs:
 * what the buyer sees right now. Shared by the tab and the room's own page,
 * which passes no action.
 */
export function StateBanner({
  room,
  action,
}: Readonly<{ room: DealRoom; action?: ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const notice = (claim: string, tone?: CalloutTone, withAction = true) => (
    <Callout
      tone={tone}
      kind="standing"
      title={claim}
      actions={withAction ? action : undefined}
    />
  );
  switch (room.state) {
    case "paused":
      return notice(t("roompage.banner.paused"), "warning");
    case "closed":
      return notice(t("roompage.banner.closed"));
    case "expired":
      return notice(t("roompage.banner.expired"), "warning");
    case "archived":
      return notice(t("roompage.banner.archived"), "danger");
    default:
      return room.expires_at
        ? notice(
            t("roompage.banner.liveUntil", {
              when: formatDateAbbrev(room.expires_at, locale, recordZone),
            }),
            undefined,
            false,
          )
        : null;
  }
}

// What still stands between a live room and one worth opening, read from the
// room's own records. It goes away once all four are done.
export function RoomSetup({
  room,
  onEditText,
  onInvite,
}: Readonly<{ room: DealRoom; onEditText: () => void; onInvite: () => void }>) {
  const t = useT();
  const participants = useParticipants(room.id);
  const documents = useRoomDocuments(room.id);
  const threads = useRoomThreads(room.id);
  if (!participants.isSuccess || !documents.isSuccess || !threads.isSuccess) {
    return null;
  }
  const done = [
    Boolean(room.welcome_message),
    participants.data.data.some((p) => !p.revoked_at),
    documents.data.data.length > 0,
    threads.data.data.length > 0,
  ];
  const doneCount = done.filter(Boolean).length;
  if (doneCount === done.length) {
    return null;
  }
  const current = done.indexOf(false);
  const stateOf = (i: number) =>
    done[i] ? "done" : i === current ? "current" : "open";
  const verb = (i: number, label: string, onClick: () => void) =>
    done[i] ? undefined : (
      <Button variant={i === current ? "primary" : "ghost"} onClick={onClick}>
        {label}
      </Button>
    );
  return (
    <Panel title={t("roomtab.setup.title")}>
      <PanelBody>
        <Checklist count={{ done: doneCount, total: done.length }}>
          <ChecklistStep
            state={stateOf(0)}
            title={t("roomtab.setup.text")}
            caption={t("roomtab.setup.textHint")}
            action={verb(0, t("roomtab.setup.editText"), onEditText)}
          />
          <ChecklistStep
            state={stateOf(1)}
            title={t("roomtab.setup.invite")}
            caption={t("roomtab.setup.inviteHint")}
            action={verb(1, t("access.invite"), onInvite)}
          />
          <ChecklistStep
            state={stateOf(2)}
            title={t("roomtab.setup.document")}
            caption={t("roomtab.setup.documentHint")}
          />
          <ChecklistStep
            state={stateOf(3)}
            title={t("roomtab.setup.thread")}
            caption={t("roomtab.setup.threadHint")}
          />
        </Checklist>
      </PanelBody>
    </Panel>
  );
}

// Who is in the room, under its name: how many were invited, how many have
// been through the door, and when a buyer last looked. The counts come from
// the participants read the access panel already makes, so no second request.
//
// The line says how many and the panel says who, so the two are not one fact
// said twice.
export function RoomFacts({ room }: Readonly<{ room: DealRoom }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
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
        <span>
          {t("room.card.lastSeen", {
            when: formatDateAbbrev(lastSeen, locale, recordZone),
          })}
        </span>
      ) : null}
    </p>
  );
}
