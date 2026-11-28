// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RotateCw } from "lucide-react";
import { useId, useState } from "react";
import { Badge, Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { IconAction } from "../../design-system/iconaction";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessengerContext, MessengerScope } from "./messenger.queries";
import { MessengerTeamComparison } from "./messenger.teamcomparison";

export function MessengerAudience({
  context,
  scope,
  changed,
  onReview,
  onRefresh,
  refreshing,
}: Readonly<{
  context: MessengerContext;
  scope: MessengerScope;
  changed: boolean;
  onReview: () => void;
  onRefresh?: () => void;
  refreshing?: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const title = useId();
  const [open, Open] = useState(false);
  const [shown, Shown] = useState(100);
  const room = context.conversation;
  return (
    <>
      <div className="messenger-audience-head">
        <span>
          {room.name} ·{" "}
          {room.access === "workspace"
            ? t("messenger.accessWorkspace")
            : plural("messenger.audienceCount", room.members.length, {
                count: formatNumber(room.members.length, locale),
              })}
        </span>
        <Button variant="ghost" onClick={() => Open(true)}>
          {t("messenger.viewAudience")}
        </Button>
        {onRefresh && (
          <IconAction
            inline
            label={t("messenger.refresh")}
            icon={<RotateCw aria-hidden="true" />}
            disabled={refreshing}
            onClick={onRefresh}
          />
        )}
      </div>
      {room.access !== "workspace" && (
        <p>{t(accessLabel(room.access, room.kind))}</p>
      )}
      {changed && (
        <Callout tone="warning" title={t("messenger.audienceChanged")}>
          <Button variant="ghost" onClick={() => Open(true)}>
            {t("messenger.reviewAudience")}
          </Button>
        </Callout>
      )}
      {room.external && <p>{t("messenger.externalAudience")}</p>}
      <Modal
        intent="drawer-reading"
        open={open}
        onClose={() => Open(false)}
        labelledBy={title}
      >
        <Heading className="modal-title" id={title} size="medium">
          {t("messenger.viewAudience")}
        </Heading>
        <p>{t(accessLabel(room.access, room.kind))}</p>
        {room.access === "workspace" && <p>{t("messenger.joinedOnly")}</p>}
        {open && (
          <MessengerTeamComparison
            members={context.conversation.members}
            access={context.conversation.access}
            scope={scope}
          />
        )}
        <ul className="messenger-audience-list">
          {room.members.slice(0, shown).map((member) => (
            <li key={member.id}>
              <span>{member.name}</span>
              <small className="t-caption">{member.email || member.id}</small>
              {member.external && (
                <Badge tone="warning">{t("messenger.externalMember")}</Badge>
              )}
              {member.bot && (
                <Badge tone="warning">{t("messenger.appMember")}</Badge>
              )}
              {!member.active && (
                <Badge tone="warning">{t("messenger.inactiveMember")}</Badge>
              )}
            </li>
          ))}
        </ul>
        {shown < room.members.length && (
          <Button onClick={() => Shown((value) => value + 100)}>
            {t("messenger.more")}
          </Button>
        )}
        {changed && (
          <Button
            variant="primary"
            disabled={!room.audience_known || room.external}
            onClick={() => {
              onReview();
              Open(false);
            }}
          >
            {t("messenger.useAudience")}
          </Button>
        )}
        <Button onClick={() => Open(false)}>{t("common.close")}</Button>
      </Modal>
    </>
  );
}

function accessLabel(access: string | undefined, kind: string) {
  switch (access) {
    case "workspace":
      return "messenger.accessWorkspace";
    case "team":
      return "messenger.accessTeam";
    case "private":
      return "messenger.accessPrivate";
    case "participants":
      return "messenger.accessParticipants";
    default:
      return kind === "channel"
        ? "messenger.accessUnknown"
        : "messenger.accessParticipants";
  }
}
