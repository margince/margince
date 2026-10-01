// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarCheck, Clock, MapPin, Video } from "lucide-react";
import type { FormEvent, ReactNode } from "react";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import {
  Button,
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import {
  formatDateTime,
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { formatDayFull, formatTimeRange } from "../format/meetingtime";
import { useLocale, useT } from "../i18n";
import { BookingZone } from "./booking-common";
import type { GuestSlot } from "./booking-guest-month";
import {
  PROVIDER_VIDEO_APP,
  VIDEO_APP_NAME,
  type VideoApp,
} from "./booking-video";

/**
 * The app a booking's video link will come from, as the guest page names it.
 *
 * The public read states it (`video_app`) and is the authority. The host's own
 * profile, which the preview reads, carries only the provider and the switch
 * ("absent means on"), so the preview mirrors the server's rule to show the
 * host what a guest will see.
 */
export function videoAppOf(
  profile: Readonly<{
    video_app?: VideoApp;
    provider?: string;
    video_call?: boolean;
  }>,
  preview: boolean,
): VideoApp | undefined {
  if (!preview || profile.video_call === false) return profile.video_app;
  const provider = profile.provider;
  return provider === "gcal" || provider === "graphcal"
    ? PROVIDER_VIDEO_APP[provider]
    : undefined;
}

function HostLine({
  icon,
  children,
}: Readonly<{ icon: ReactNode; children: ReactNode }>) {
  return (
    <p className="bookguest-line">
      {icon}
      <span>{children}</span>
    </p>
  );
}

/** Who the guest is meeting, for how long, where, and in which zone. */
export function GuestHost({
  host,
  videoApp,
  selected,
  zone,
  onZone,
  children,
}: Readonly<{
  host: Readonly<{
    host_name: string;
    title: string;
    duration_minutes: number;
    location: string;
  }>;
  videoApp?: VideoApp;
  selected: GuestSlot | null;
  zone: string;
  onZone: (zone: string) => void;
  children?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <section className="bookguest-host">
      <p className="t-caption">{host.host_name}</p>
      <Heading as="h1" size="large">
        {host.title}
      </Heading>
      <HostLine icon={<Clock aria-hidden="true" />}>
        {t("co.recent.minutes", {
          count: formatNumber(host.duration_minutes, locale),
        })}
      </HostLine>
      {videoApp ? (
        <HostLine icon={<Video aria-hidden="true" />}>
          {t("scheduling.videoApp", { app: VIDEO_APP_NAME[videoApp] })}
        </HostLine>
      ) : (
        host.location && (
          <HostLine icon={<MapPin aria-hidden="true" />}>
            {host.location}
          </HostLine>
        )
      )}
      {selected && (
        <HostLine icon={<CalendarCheck aria-hidden="true" />}>
          {formatDayFull(selected.start, locale, zone)} ·{" "}
          {formatTimeRange(selected.start, selected.end, locale, zone)}
        </HostLine>
      )}
      {children}
      <div className="bookguest-zone">
        <BookingZone value={zone} onChange={onZone} />
      </div>
    </section>
  );
}

export type GuestDetails = Readonly<{
  name: string;
  email: string;
  topic: string;
  consent: boolean;
}>;

/**
 * The guest's details for the time they picked. A personal proposal already
 * names its recipient, so it asks only for consent.
 */
export function GuestDetailsForm({
  selected,
  zone,
  personal,
  details,
  onDetails,
  onChangeTime,
  onSubmit,
  refused,
  pending,
  error,
}: Readonly<{
  selected: GuestSlot;
  zone: string;
  personal: boolean;
  details: GuestDetails;
  onDetails: (next: GuestDetails) => void;
  onChangeTime: () => void;
  onSubmit: () => void;
  refused: boolean;
  pending: boolean;
  error: unknown;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const set = (edit: Partial<GuestDetails>) =>
    onDetails({ ...details, ...edit });
  return (
    <form
      className="bookguest-form"
      onSubmit={(event: FormEvent) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <Heading as="h2" size="medium">
        {t("scheduling.book")}
      </Heading>
      {!personal && (
        <>
          <Field label={t("book.name")}>
            {(control) => (
              <TextInput
                {...control}
                required
                autoComplete="name"
                value={details.name}
                onChange={(e) => set({ name: e.target.value })}
              />
            )}
          </Field>
          <Field label={t("book.email")}>
            {(control) => (
              <TextInput
                {...control}
                required
                type="email"
                autoComplete="email"
                value={details.email}
                onChange={(e) => set({ email: e.target.value })}
              />
            )}
          </Field>
          <Field label={t("scheduling.guestAgenda")}>
            {(control) => (
              <Textarea
                {...control}
                value={details.topic}
                onChange={(e) => set({ topic: e.target.value })}
              />
            )}
          </Field>
        </>
      )}
      <Checkbox
        checked={details.consent}
        onChange={(event) => set({ consent: event.target.checked })}
        label={t("book.consentWording")}
      />
      <div className="bookguest-actions">
        <Button onClick={onChangeTime}>{t("scheduling.changeTime")}</Button>
        <Button
          type="submit"
          variant="primary"
          disabled={refused || !details.consent}
          pending={pending}
        >
          {t("scheduling.confirmAt", {
            day: formatDayMonth(selected.start, locale, zone),
            time: formatTimeOfDay(selected.start, locale, zone),
          })}
        </Button>
      </div>
      <ErrorLine error={error} />
    </form>
  );
}

/** What the host sees above their own page's preview: that nothing here sends. */
export function PreviewNotice({ enabled }: Readonly<{ enabled: boolean }>) {
  const t = useT();
  return (
    <Panel title={t("scheduling.preview")} tone="accent">
      <PanelBody>
        <p>
          {t(enabled ? "scheduling.previewActive" : "scheduling.previewPaused")}
        </p>
        <a href="#/settings/meetings">{t("scheduling.openSettings")}</a>
      </PanelBody>
    </Panel>
  );
}

/**
 * A paused page, or a personal link already used: the meeting it booked when
 * there is one, and otherwise the plain fact that nothing can be booked here.
 */
export function GuestUnavailable({
  host,
  zone,
}: Readonly<{
  host: Readonly<{
    proposal: components["schemas"]["PublicMeetingProposal"] | null;
  }>;
  zone: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const meeting = host.proposal?.meeting;
  const token = meeting?.management_token;
  return (
    <Panel>
      <PanelBody>
        {meeting && token ? (
          <>
            <Heading as="h1" size="large">
              {meeting.subject}
            </Heading>
            <p>{formatDateTime(meeting.start, locale, zone)}</p>
            <p className="t-caption">{t("scheduling.savedRequest")}</p>
            <Button
              variant="primary"
              onClick={() =>
                navigate({ screen: "book", id: `manage-${token}` })
              }
            >
              {t("scheduling.openMeeting")}
            </Button>
          </>
        ) : (
          <p>{t("scheduling.unavailable")}</p>
        )}
      </PanelBody>
    </Panel>
  );
}
