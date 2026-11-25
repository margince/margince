// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarCheck, Clock, MapPin, Video } from "lucide-react";
import type { FormEvent, ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import {
  Button,
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import type { ISODay } from "../../design-system/calendar";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { MeetingSlots } from "../../design-system/meetingslots";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { SegmentedControl } from "../../design-system/segmentedcontrol";
import {
  formatDateTime,
  formatDayLong,
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
  identifierNumber,
} from "../../format/format";
import { formatDayFull, formatTimeRange } from "../../format/meetingtime";
import { useLocale, useT } from "../../i18n";
import {
  PROVIDER_VIDEO_APP,
  VIDEO_APP_NAME,
  type VideoApp,
} from "../../lib/booking-video";
import { navigate } from "../../lib/router";
import { BookingZone } from "./booking-common";
import type { GuestSlot, monthDays } from "./booking-guest-month";

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
  lengths,
  length = host.duration_minutes,
  onLength,
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
  // The lengths a guest may pick between, the default first; with one or none
  // the page states its length instead.
  lengths?: readonly number[];
  length?: number;
  onLength?: (length: number) => void;
  selected: GuestSlot | null;
  zone: string;
  onZone: (zone: string) => void;
  children?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const minutes = (length: number) =>
    t("co.recent.minutes", { count: formatNumber(length, locale) });
  return (
    <section className="bookguest-host">
      <p className="t-caption">{host.host_name}</p>
      <Heading as="h1" size="large">
        {host.title}
      </Heading>
      {lengths && lengths.length > 1 && onLength ? (
        <div className="bookguest-line">
          <Clock aria-hidden="true" />
          <SegmentedControl
            label={t("book.lengthChoice")}
            options={lengths.map(identifierNumber)}
            labels={Object.fromEntries(
              lengths.map((option) => [
                identifierNumber(option),
                minutes(option),
              ]),
            )}
            value={identifierNumber(length)}
            onChange={(next) => onLength(Number(next))}
          />
        </div>
      ) : (
        <HostLine icon={<Clock aria-hidden="true" />}>
          {minutes(length)}
        </HostLine>
      )}
      {videoApp ? (
        <HostLine icon={<Video aria-hidden="true" />}>
          {t("book.videoApp", { app: VIDEO_APP_NAME[videoApp] })}
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
          <Field label={t("book.agenda")}>
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
        <AppLink href="#/settings/meetings">
          {t("scheduling.openSettings")}
        </AppLink>
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
    host_name: string;
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
            <p className="t-caption">{t("book.savedRequest")}</p>
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
          <p>
            {host.host_name
              ? t("scheduling.pausedUnavailable", { host: host.host_name })
              : t("book.unavailable")}
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}

// The tab names the host only while their page takes bookings; a paused or
// missing page keeps a generic title, as its link-preview card does.
export function guestTitle(
  t: ReturnType<typeof useT>,
  host: { paused: boolean; host_name: string } | undefined,
): string {
  return host && !host.paused && host.host_name
    ? t("book.pageTitle", { host: host.host_name })
    : t("book.pageTitleGeneric");
}

/**
 * The right-hand column before a time is picked: the times a personal
 * proposal offers first, then the chosen day's free times.
 */
export function GuestTimes({
  offered,
  day,
  days,
  times,
  monthKey,
  zone,
  needsCalendar,
  slots,
  onSelect,
}: Readonly<{
  offered: readonly GuestSlot[];
  day: ISODay | "";
  days: ReturnType<typeof monthDays> | undefined;
  times: readonly GuestSlot[];
  monthKey: string;
  zone: string;
  needsCalendar: boolean;
  slots: QueryLike<unknown>;
  onSelect: (slot: GuestSlot) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const monthHasTimes = days?.free.some((free) => free.startsWith(monthKey));
  return (
    <>
      {offered.length > 0 && (
        <div className="bookguest-suggested">
          <Heading as="h2" size="medium">
            {t("scheduling.suggestedTimes")}
          </Heading>
          <MeetingSlots
            slots={offered.map((slot) => ({
              ...slot,
              label: formatDateTime(slot.start, locale, zone),
            }))}
            onSelect={onSelect}
            empty={t("book.noTimes")}
          />
        </div>
      )}
      <Heading as="h2" size="medium">
        {day ? formatDayLong(day, locale, zone) : t("scheduling.chooseTime")}
      </Heading>
      {needsCalendar ? (
        <p className="t-caption">{t("scheduling.previewCalendarSetup")}</p>
      ) : (
        <QueryGate pendingLabel={t("common.loading")} query={slots}>
          {() => (
            <MeetingSlots
              slots={times.map((slot) => ({
                ...slot,
                label: formatTimeOfDay(slot.start, locale, zone),
              }))}
              onSelect={onSelect}
              empty={t(monthHasTimes ? "book.noTimesDay" : "book.noTimesMonth")}
            />
          )}
        </QueryGate>
      )}
    </>
  );
}
