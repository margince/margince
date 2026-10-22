// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Copy } from "lucide-react";
import { useRef, useState } from "react";
import type { components } from "../../api/schema";
import { Button, EmptyState, OverflowMenu } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { PROVIDER_VIDEO_APP, VIDEO_APP_NAME } from "../../lib/booking-video";
import { navigate } from "../../lib/router";
import { usePatchSchedulingProfile } from "../../lib/scheduling-profile-query";
import {
  type CalendarStanding,
  calendarName,
  needsAttention,
  useCalendarConnect,
  useCalendarStanding,
} from "./booking-calendar-state";
import { AVAILABILITY_ID } from "./meeting-availability";
import { ConnectRefusal } from "./meeting-calendar";
import {
  MeetingLinkDrawer,
  ReplaceAddressConfirm,
  videoOf,
} from "./meeting-link-drawer";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];
type CaptureConnection = components["schemas"]["CaptureConnection"];

// Why Accept bookings cannot be turned on yet. Turning it off is never refused:
// pausing is the way out of every broken state below.
const ENABLE_REFUSAL: Partial<Record<CalendarStanding, MessageKey>> = {
  none: "scheduling.acceptNeedsCalendar",
  unsaved: "scheduling.acceptNeedsChoice",
  expired: "scheduling.acceptNeedsReconnect",
  readOnly: "scheduling.acceptNeedsReconnect",
};

/**
 * The host's booking link: whether it takes bookings, the link itself and the
 * verbs on it. One row today, because a host has one booking page; the table
 * is the shape several links will grow into.
 */
export function BookingLinksCard({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const { standing, connection } = useCalendarStanding(profile);
  const attention = needsAttention(profile, standing);
  const write = usePatchSchedulingProfile();
  const [editing, setEditing] = useState(false);
  const [replacing, setReplacing] = useState(false);
  const toHours = useRef(false);
  const refusal = profile.enabled ? undefined : ENABLE_REFUSAL[standing];
  let status: Readonly<{ kind: StatusKind; key: MessageKey }> = {
    kind: "neutral",
    key: "scheduling.status.paused",
  };
  if (attention)
    status = { kind: "attention", key: "scheduling.status.attention" };
  else if (profile.enabled) status = { kind: "done", key: "scheduling.active" };
  return (
    <Panel
      title={t("scheduling.linksTitle")}
      titleAction={<StatusBadge kind={status.kind} label={t(status.key)} />}
      footer={
        <span className="t-caption">{t("scheduling.personalLinksFoot")}</span>
      }
    >
      <PanelBody className="form-stack">
        {attention && (
          <NoInvitationsCallout
            standing={standing}
            connection={connection}
            pausing={write.isPending}
            onPause={() => write.mutate({ enabled: false })}
          />
        )}
        <SettingList>
          <SettingRow
            label={t("scheduling.acceptBookings")}
            description={t(refusal ?? "scheduling.acceptBookingsHelp")}
            control={(control) => (
              <Switch
                label={t("scheduling.acceptBookings")}
                labelHidden
                describedBy={control["aria-describedby"]}
                checked={profile.enabled}
                disabled={refusal !== undefined || standing === "loading"}
                pending={write.isPending}
                onChange={(enabled) => write.mutate({ enabled })}
              />
            )}
          />
        </SettingList>
        <ErrorLine error={write.error} />
      </PanelBody>
      {profile.slug ? (
        <DataTable
          bleed
          fold
          label={t("scheduling.linksTitle")}
          rows={[profile]}
          rowKey={(row) => row.slug ?? ""}
          columns={[
            {
              key: "link",
              header: t("scheduling.col.link"),
              grow: true,
              fold: "title",
              render: (row) => <LinkCell profile={row} />,
            },
            {
              key: "length",
              header: t("scheduling.col.length"),
              render: (row) => <LengthCell minutes={row.duration_minutes} />,
            },
            {
              key: "video",
              header: t("scheduling.videoCall"),
              render: (row) => videoText(t, row),
            },
            {
              key: "status",
              header: t("scheduling.col.status"),
              render: (row) => (
                <RowStatus profile={row} attention={attention} />
              ),
            },
            {
              key: "actions",
              header: t("scheduling.col.actions"),
              headerHidden: true,
              align: "end",
              fold: "trail",
              render: (row) => (
                <LinkActions
                  profile={row}
                  onEdit={() => {
                    toHours.current = false;
                    setEditing(true);
                  }}
                  onReplace={() => setReplacing(true)}
                />
              ),
            },
          ]}
        />
      ) : (
        <PanelBody>
          <EmptyState
            title={t("scheduling.noLinkYet")}
            action={
              <Button
                variant="primary"
                pending={write.isPending}
                onClick={() => write.mutate({ enabled: false })}
              >
                {t("scheduling.createLink")}
              </Button>
            }
          >
            {t("scheduling.noLinkYetBody")}
          </EmptyState>
        </PanelBody>
      )}
      <MeetingLinkDrawer
        open={editing}
        onClose={() => setEditing(false)}
        profile={profile}
        onChangeHours={() => {
          toHours.current = true;
          setEditing(false);
        }}
        returnFocusTo={() =>
          toHours.current ? document.getElementById(AVAILABILITY_ID) : null
        }
      />
      <ReplaceAddressConfirm
        open={replacing}
        onClose={() => setReplacing(false)}
      />
    </Panel>
  );
}

// The video column, in words: the calendar's own app, the host's own place,
// none, or not set while no calendar names an app.
function videoText(t: Translator, profile: Profile): string {
  const video = videoOf(profile);
  if (video === "own") return t("scheduling.video.ownShort");
  if (video === "none") return t("scheduling.video.none");
  if (profile.provider === "") return t("scheduling.videoNotSet");
  return VIDEO_APP_NAME[PROVIDER_VIDEO_APP[profile.provider]];
}

function LinkCell({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  return (
    <CellStack>
      <span className="t-name">
        {profile.title}{" "}
        <span className="t-caption">{t("scheduling.mainLink")}</span>
      </span>
      <span className="t-caption meeting-link-url">
        {profile.public_url ?? t("scheduling.publicUrlMissing")}
      </span>
    </CellStack>
  );
}

function LengthCell({ minutes }: Readonly<{ minutes: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return plural("scheduling.minutes", minutes, {
    count: formatNumber(minutes, locale),
  });
}

function RowStatus({
  profile,
  attention,
}: Readonly<{ profile: Profile; attention: boolean }>) {
  const t = useT();
  if (attention)
    return (
      <StatusBadge kind="attention" label={t("scheduling.row.noInvitations")} />
    );
  return profile.enabled ? (
    <StatusBadge kind="done" label={t("scheduling.row.live")} />
  ) : (
    <StatusBadge kind="neutral" label={t("scheduling.status.paused")} />
  );
}

function LinkActions({
  profile,
  onEdit,
  onReplace,
}: Readonly<{ profile: Profile; onEdit: () => void; onReplace: () => void }>) {
  const t = useT();
  const copy = useClipboardCopy(profile.public_url ?? "", {
    copy: t("scheduling.copyLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  return (
    <div className="cell-actions">
      <Button disabled={!profile.public_url} onClick={copy.copy}>
        <Copy aria-hidden />
        {copy.label}
      </Button>
      <OverflowMenu
        label={t("scheduling.linkActions", { name: profile.title })}
      >
        <Button onClick={onEdit}>{t("scheduling.editLink")}</Button>
        <Button onClick={() => navigate({ screen: "book", id: "preview" })}>
          {t("scheduling.preview")}
        </Button>
        <Button onClick={onReplace}>{t("scheduling.replaceAddress")}</Button>
      </OverflowMenu>
      {copy.notice}
    </div>
  );
}

/** Bookings are live and nothing lands in a calendar: say so, with both ways out. */
function NoInvitationsCallout({
  standing,
  connection,
  pausing,
  onPause,
}: Readonly<{
  standing: CalendarStanding;
  connection: CaptureConnection | undefined;
  pausing: boolean;
  onPause: () => void;
}>) {
  const t = useT();
  const calendar = useCalendarConnect();
  const known =
    (standing === "expired" || standing === "readOnly") &&
    (connection?.provider === "gcal" || connection?.provider === "graphcal")
      ? connection.provider
      : null;
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={t("scheduling.liveNoInvitesTitle")}
      actions={
        <>
          <Button
            variant="primary"
            pending={calendar.connect.isPending}
            onClick={() => calendar.start(known ?? calendar.target)}
          >
            {t("scheduling.reconnectCalendar")}
          </Button>
          <Button pending={pausing} onClick={onPause}>
            {t("scheduling.pause")}
          </Button>
        </>
      }
    >
      <p>{attentionBody(t, standing, connection)}</p>
      <ConnectRefusal connect={calendar.connect} />
    </Callout>
  );
}

function attentionBody(
  t: Translator,
  standing: CalendarStanding,
  connection: CaptureConnection | undefined,
): string {
  const provider = calendarName(t, connection?.provider ?? "");
  if (standing === "expired")
    return t("scheduling.attentionExpired", { provider });
  if (standing === "readOnly")
    return t("scheduling.attentionReadOnly", { provider });
  return t("scheduling.attentionMissing");
}
