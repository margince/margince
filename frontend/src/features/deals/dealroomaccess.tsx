import { Link2 } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { humanizeToken } from "../settings/audit";
import { CAPABILITIES, CAPABILITY_LABELS } from "./dealroomcapability";
import "./dealroomaccess.css";
import { SurfaceState } from "../../design-system/surfacestate";
import {
  CapabilityDialog,
  InviteDialog,
  ReissueDialog,
  RevokeDialog,
} from "./dealroomaccess.dialogs";
// Who may enter the room, and the verbs that change it: invite, issue a new
// link, change what a contact may do, revoke. Every link is shown once, with
// Copy: the server never stores it in clear, and pasting it is the normal path.
//
// Capability is a required, explained choice. The API defaults to `view`,
// and a buyer invited with the default could read the documents and never be
// able to say anything about them.
import { useParticipants } from "./dealroomaccess.queries";

export { buyerLink } from "./dealroomaccess.dialogs";
export {
  participantsKey,
  refreshSeats,
  useParticipants,
  useRoomAttendance,
} from "./dealroomaccess.queries";

export type DealRoom = components["schemas"]["DealRoom"];

export type Participant = components["schemas"]["DealRoomParticipant"];

export type Capability = components["schemas"]["DealRoomParticipantCapability"];

export type Issued = components["schemas"]["DealRoomInvitationIssued"];

export function DealRoomAccess({
  room,
  mayManage,
}: Readonly<{ room: DealRoom; mayManage: boolean }>) {
  const t = useT();
  const [inviting, setInviting] = useState(false);
  const participants = useParticipants(room.id);
  const rows = participants.data?.data ?? [];
  return (
    <Panel
      title={t("access.title")}
      titleAction={
        mayManage ? (
          <Button onClick={() => setInviting(true)}>
            {t("access.invite")}
          </Button>
        ) : undefined
      }
    >
      <QueryStates
        query={participants}
        pendingLines={2}
        pendingLabel={t("access.title")}
      >
        {rows.length === 0 ? (
          <PanelBody>
            <SurfaceState
              state="empty"
              emptyLabel={t("access.empty")}
              loadingLabel={t("access.title")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        ) : (
          rows.map((p) => (
            <ParticipantRow
              key={p.id}
              room={room}
              participant={p}
              mayManage={mayManage}
            />
          ))
        )}
      </QueryStates>
      <InviteDialog
        room={room}
        open={inviting}
        onClose={() => setInviting(false)}
      />
    </Panel>
  );
}

// What this contact has actually done in the room, under the line that says
// whether they have been here. A seat that has taken nothing says nothing:
// "0 documents" reads as a judgement about the buyer, and the honest state
// early in a room's life is simply that there is nothing to report yet.
function ReadingSoFar({ participant }: Readonly<{ participant: Participant }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const downloads = participant.download_count ?? 0;
  if (downloads === 0) {
    return null;
  }
  const titles = participant.documents_downloaded ?? [];
  return (
    <p className="t-caption">
      {plural("access.downloads", downloads, {
        count: formatNumber(downloads, locale),
      })}
      {titles.length > 0 ? ` · ${titles.join(", ")}` : ""}
    </p>
  );
}

function ParticipantRow({
  room,
  participant,
  mayManage,
}: Readonly<{ room: DealRoom; participant: Participant; mayManage: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [confirming, setConfirming] = useState<
    "revoke" | "reissue" | "capability" | null
  >(null);
  const revoked =
    participant.revoked_at !== null && participant.revoked_at !== undefined;
  const capability = CAPABILITIES.find((c) => c === participant.capability);
  return (
    <PanelRow
      className={revoked ? "access-row access-row-revoked" : "access-row"}
    >
      <div className="access-row-main">
        <p>
          {participant.full_name}
          <span className="t-caption"> · {participant.email}</span>
        </p>
        <p className="t-caption">
          {capability
            ? t(CAPABILITY_LABELS[capability])
            : humanizeToken(participant.capability)}
          {" · "}
          {revoked
            ? t("access.state.revoked")
            : participant.has_signed_in
              ? t("access.state.active")
              : t("access.state.invited")}
          {participant.last_seen_at
            ? ` · ${t("access.lastSeen", { when: formatDateAbbrev(participant.last_seen_at, locale, recordZone) })}`
            : ""}
        </p>
        <ReadingSoFar participant={participant} />
        {participant.link_requested_at && !revoked ? (
          <p className="t-caption access-row-request">
            <Link2 aria-hidden />
            {t("access.linkRequested", {
              when: formatDateAbbrev(
                participant.link_requested_at,
                locale,
                recordZone,
              ),
            })}
          </p>
        ) : null}
      </div>
      <div className="access-row-side">
        {revoked ? <Badge>{t("access.state.revokedBadge")}</Badge> : null}
        {mayManage && !revoked ? (
          <OverflowMenu
            label={t("table.rowActions", { name: participant.full_name })}
          >
            <Button variant="ghost" onClick={() => setConfirming("reissue")}>
              {t("access.issueLink")}
            </Button>
            <Button variant="ghost" onClick={() => setConfirming("capability")}>
              {t("access.changeCapability")}
            </Button>
            <Button variant="ghost" onClick={() => setConfirming("revoke")}>
              {t("access.revoke")}
            </Button>
          </OverflowMenu>
        ) : null}
      </div>
      <RevokeDialog
        room={room}
        participant={participant}
        open={confirming === "revoke"}
        onClose={() => setConfirming(null)}
      />
      <ReissueDialog
        room={room}
        participant={participant}
        open={confirming === "reissue"}
        onClose={() => setConfirming(null)}
      />
      <CapabilityDialog
        room={room}
        participant={participant}
        open={confirming === "capability"}
        onClose={() => setConfirming(null)}
      />
    </PanelRow>
  );
}
