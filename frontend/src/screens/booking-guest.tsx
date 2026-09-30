import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { navigate } from "../app/router";
import { Calendar, type ISODay } from "../design-system/calendar";
import { CompanyLogo } from "../design-system/companylogo";
import { Heading } from "../design-system/heading";
import { MeetingSlots } from "../design-system/meetingslots";
import { Panel, PanelBody } from "../design-system/panel";
import {
  formatDateTime,
  formatDayLong,
  formatTimeOfDay,
} from "../format/format";
import { dayInZone, viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { BookingFooter, useBookingIntent } from "./booking-common";
import { throwBookingProblem } from "./booking-errors";
import {
  dayRefused,
  type GuestSlot,
  isoMonth,
  monthDays,
  monthOf,
  monthWindow,
  readMonth,
} from "./booking-guest-month";
import {
  type GuestDetails,
  GuestDetailsForm,
  GuestHost,
  GuestUnavailable,
  PreviewNotice,
  videoAppOf,
} from "./booking-guest-parts";
import { QueryGate, type QueryLike, throwProblem } from "./common";
import "./booking-guest.css";

export const PUBLIC_BOOKING_CONSENT = { policy_version: "2026-07" };
const NO_DETAILS: GuestDetails = {
  name: "",
  email: "",
  topic: "",
  consent: false,
};

export function BookingGuestScreen({
  hostSlug,
  proposalToken,
  preview = false,
}: Readonly<{ hostSlug: string; proposalToken?: string; preview?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const [zone, setZone] = useState(viewerZone);
  const intent = useBookingIntent();
  const [now] = useState(() => Date.now());
  const [month, setMonth] = useState(() => {
    const today = new Date(now);
    return new Date(today.getFullYear(), today.getMonth(), 1);
  });
  const [pickedDay, setPickedDay] = useState<ISODay | "">("");
  const [selected, setSelected] = useState<GuestSlot | null>(null);
  const [details, setDetails] = useState(NO_DETAILS);
  const profile = useQuery({
    queryKey: preview
      ? ["scheduling-profile"]
      : ["public-booking-profile", hostSlug, proposalToken],
    select: (value) => ({
      ...value,
      host_name: value.host_name ?? "",
      company_name: value.company_name ?? "",
      proposal: "proposal" in value ? value.proposal : null,
      video_app: videoAppOf(value, preview),
    }),
    queryFn: async () => {
      if (preview) {
        const { data, error } = await api.GET("/scheduling/profile");
        if (error) throwProblem(error);
        return data;
      }
      if (proposalToken) {
        const { data, error } = await api.GET("/public/proposal/{token}", {
          params: { path: { token: proposalToken } },
        });
        if (error) throwProblem(error);
        return { ...data.profile, proposal: data };
      }
      const { data, error } = await api.GET(
        "/public/booking/{host_slug}/profile",
        { params: { path: { host_slug: hostSlug } } },
      );
      if (error) throwProblem(error);
      return { ...data, proposal: null };
    },
  });
  const needsCalendar =
    preview &&
    profile.isSuccess &&
    !("provider" in profile.data && profile.data.provider);
  const visible = monthWindow(month, zone, now);
  const read = async (from: string, to: string) => {
    if (preview) {
      const { data, error } = await api.GET("/availability", {
        params: { query: { from, to, reliable: true } },
      });
      if (error) throwBookingProblem(error, t);
      return data;
    }
    if (proposalToken) {
      const { data, error } = await api.GET(
        "/public/proposal/{token}/availability",
        { params: { path: { token: proposalToken }, query: { from, to } } },
      );
      if (error) throwBookingProblem(error, t);
      return data;
    }
    const { data, error } = await api.GET(
      "/public/booking/{host_slug}/availability",
      { params: { path: { host_slug: hostSlug }, query: { from, to } } },
    );
    if (error) throwBookingProblem(error, t);
    return data;
  };
  const slots = useQuery({
    queryKey: [
      preview ? "reliable-availability" : "public-booking-slots",
      preview ? "booking-preview" : hostSlug,
      proposalToken,
      visible?.from,
      visible?.to,
      locale,
      preview ? profile.data : undefined,
    ],
    enabled:
      profile.isSuccess && !needsCalendar && (preview || profile.data.enabled),
    // A month already over has no times left to offer, and says so rather
    // than asking the server about the past.
    queryFn: () =>
      visible ? readMonth(read, visible) : { slots: [], truncated: false },
  });
  const book = useMutation({
    mutationFn: async (input: {
      slug: string;
      name: string;
      email: string;
      topic: string;
      start: string;
      end: string;
      wording: string;
    }) => {
      if (proposalToken) {
        const { data, error } = await api.POST("/public/proposal/{token}", {
          params: {
            path: { token: proposalToken },
          },
          body: {
            start: input.start,
            end: input.end,
            consent: { ...PUBLIC_BOOKING_CONSENT, wording: input.wording },
          },
        });
        if (error) throwProblem(error);
        return data;
      }
      const { data, error } = await api.POST("/public/booking/{host_slug}", {
        params: {
          path: { host_slug: input.slug },
          header: { "Idempotency-Key": intent(input) },
        },
        body: {
          start: input.start,
          end: input.end,
          booker: { name: input.name, email: input.email },
          subject: input.topic,
          delivery: "calendar",
          consent: { ...PUBLIC_BOOKING_CONSENT, wording: input.wording },
        },
      });
      if (error) throwProblem(error);
      if (!data.invitation) throw new Error(t("scheduling.deliveryUnknown"));
      return data.invitation;
    },
    onSuccess: (value) => {
      if (value.management_token)
        navigate({
          screen: "book",
          id: `manage-${value.management_token}`,
        });
    },
    onError: () => {
      void slots.refetch();
      void profile.refetch();
    },
  });
  const days = slots.data ? monthDays(slots.data, zone) : undefined;
  const monthKey = isoMonth(month);
  const day = pickedDay?.startsWith(monthKey)
    ? pickedDay
    : (days?.free.find((free) => free.startsWith(monthKey)) ?? "");
  const today = dayInZone(now, zone);
  const pickDay = (next: ISODay) => {
    setPickedDay(next);
    setSelected(null);
    if (!next.startsWith(monthKey)) setMonth(monthOf(next));
  };
  const publicUnavailable = !preview && !profile.data?.enabled;
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <QueryGate pendingLabel={t("common.loading")} query={profile}>
          {(host) =>
            publicUnavailable ? (
              <GuestUnavailable host={host} zone={zone} />
            ) : (
              <>
                {preview && <PreviewNotice enabled={host.enabled} />}
                {host.company_name && (
                  <header className="book-brand">
                    <CompanyLogo
                      name={host.company_name}
                      src={host.logo_url}
                      fallback={
                        <Heading as="div" size="medium">
                          {host.company_name}
                        </Heading>
                      }
                    />
                  </header>
                )}
                <Panel>
                  <PanelBody>
                    <div className="bookguest-grid">
                      <GuestHost
                        host={host}
                        videoApp={host.video_app}
                        selected={selected}
                        zone={zone}
                        onZone={setZone}
                      >
                        {host.proposal && (
                          <>
                            <p>{host.proposal.description}</p>
                            <p className="t-caption">
                              {t("scheduling.personalGuest")}
                            </p>
                          </>
                        )}
                      </GuestHost>
                      <section className="bookguest-month">
                        <Heading as="h2" size="medium">
                          {t("scheduling.pickDay")}
                        </Heading>
                        <Calendar
                          month={month}
                          onMonthChange={(next) => {
                            setMonth(next);
                            setPickedDay("");
                            setSelected(null);
                          }}
                          selected={day}
                          onSelect={pickDay}
                          today={new Date(now)}
                          locale={locale}
                          isDisabled={(candidate) =>
                            dayRefused(candidate, today, monthKey, days)
                          }
                        />
                      </section>
                      <section className="bookguest-times">
                        {selected ? (
                          <GuestDetailsForm
                            selected={selected}
                            zone={zone}
                            personal={Boolean(proposalToken)}
                            details={details}
                            onDetails={setDetails}
                            onChangeTime={() => setSelected(null)}
                            refused={preview}
                            pending={book.isPending}
                            error={book.error}
                            onSubmit={() => {
                              if (!preview && details.consent)
                                book.mutate({
                                  slug: hostSlug,
                                  name: details.name,
                                  email: details.email,
                                  topic: details.topic,
                                  ...selected,
                                  wording: t("book.consentWording"),
                                });
                            }}
                          />
                        ) : (
                          <GuestTimes
                            offered={host.proposal?.options ?? []}
                            day={day}
                            days={days}
                            monthKey={monthKey}
                            zone={zone}
                            needsCalendar={needsCalendar}
                            slots={slots}
                            onSelect={setSelected}
                          />
                        )}
                      </section>
                    </div>
                  </PanelBody>
                </Panel>
              </>
            )
          }
        </QueryGate>
        <BookingFooter />
      </div>
    </div>
  );
}

/**
 * The right-hand column before a time is picked: the times a personal
 * proposal offers first, then the chosen day's free times.
 */
function GuestTimes({
  offered,
  day,
  days,
  monthKey,
  zone,
  needsCalendar,
  slots,
  onSelect,
}: Readonly<{
  offered: readonly GuestSlot[];
  day: ISODay | "";
  days: ReturnType<typeof monthDays> | undefined;
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
            empty={t("scheduling.noTimes")}
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
              slots={(days?.byDay.get(day) ?? []).map((slot) => ({
                ...slot,
                label: formatTimeOfDay(slot.start, locale, zone),
              }))}
              onSelect={onSelect}
              empty={t(
                monthHasTimes
                  ? "scheduling.noTimesDay"
                  : "scheduling.noTimesMonth",
              )}
            />
          )}
        </QueryGate>
      )}
    </>
  );
}
