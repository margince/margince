import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { GoneNotice } from "../../components/gonenotice";
import { Calendar, type ISODay } from "../../design-system/calendar";
import { CompanyLogo } from "../../design-system/companylogo";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { browserZone, dayInZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { fromHere } from "../../lib/navigatefrom";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import {
  useBookingIntent,
  useBookingTitle,
  useGuestLength,
} from "./booking-common";
import { lengthNotOffered, throwBookingProblem } from "./booking-errors";
import {
  dayRefusal,
  dayWindow,
  type GuestSlot,
  isoMonth,
  monthDays,
  monthOf,
  monthWindow,
  pastKnown,
  readMonth,
} from "./booking-guest-month";
import {
  type GuestDetails,
  GuestDetailsForm,
  GuestHost,
  GuestTimes,
  GuestUnavailable,
  guestTitle,
  PreviewNotice,
  videoAppOf,
} from "./booking-guest-parts";
import "./booking-guest.css";
import {
  UnreadableCalendarLine,
  unreadableCalendarOf,
} from "./unreadable-calendar";

type Availability = components["schemas"]["MeetingAvailability"];

export const PUBLIC_BOOKING_CONSENT = { policy_version: "2026-07" };
const NO_DETAILS: GuestDetails = {
  name: "",
  email: "",
  topic: "",
  consent: false,
};

// A proposal and the host's preview offer no length to pick.
const NO_LENGTHS: readonly number[] = [];

// The length the page books at: the guest's pick while the page offers it,
// else the length the server opened the page at.
function bookedLength(
  page:
    | Readonly<{ lengths: readonly number[]; duration_minutes: number }>
    | undefined,
  requested: number | undefined,
): number | undefined {
  if (requested !== undefined && page?.lengths.includes(requested))
    return requested;
  return page?.duration_minutes;
}

export function BookingGuestScreen({
  hostSlug,
  proposalToken,
  preview = false,
}: Readonly<{ hostSlug: string; proposalToken?: string; preview?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  // A guest has only the browser's clock; a signed-in host previewing their
  // own page starts from their own zone.
  const readerZone = useViewerZone();
  const [zone, setZone] = useState(() =>
    preview ? readerZone : browserZone(),
  );
  const [requestedLength, pickLength] = useGuestLength(
    preview || Boolean(proposalToken),
  );
  const intent = useBookingIntent();
  const client = useQueryClient();
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
      : ["public-booking-profile", hostSlug, proposalToken, requestedLength],
    // A new length reads the page again, and the page stays in view meanwhile.
    // Another host's or proposal's page never stands in for this one.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === hostSlug &&
      previousQuery.queryKey[2] === proposalToken
        ? previous
        : undefined,
    select: (value) => ({
      ...value,
      host_name: value.host_name ?? "",
      company_name: value.company_name ?? "",
      proposal: "proposal" in value ? value.proposal : null,
      video_app: videoAppOf(value, preview),
      // A used proposal turns enabled off; the server's paused answers its
      // calendar. The host's preview has only the switch.
      paused: !value.enabled || ("paused" in value && value.paused),
    }),
    queryFn: async () => {
      if (preview) {
        return {
          ...unwrap(await api.GET("/scheduling/profile")),
          lengths: NO_LENGTHS,
        };
      }
      if (proposalToken) {
        const data = unwrap(
          await api.GET("/public/proposal/{token}", {
            params: { path: { token: proposalToken } },
          }),
        );
        return { ...data.profile, proposal: data, lengths: NO_LENGTHS };
      }
      const data = unwrap(
        await api.GET("/public/booking/{host_slug}/profile", {
          params: {
            path: { host_slug: hostSlug },
            query: { l: requestedLength },
          },
        }),
      );
      return { ...data, proposal: null, lengths: data.offered_lengths ?? [] };
    },
  });
  const needsCalendar =
    preview &&
    profile.isSuccess &&
    !("provider" in profile.data && profile.data.provider);
  const length = bookedLength(profile.data, requestedLength);
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
      {
        params: {
          path: { host_slug: hostSlug },
          query: { from, to, l: length },
        },
      },
    );
    if (error) {
      if (lengthNotOffered(error)) void profile.refetch();
      throwBookingProblem(error, t);
    }
    return data;
  };
  const readKey = [
    preview ? "reliable-availability" : "public-booking-slots",
    preview ? "booking-preview" : hostSlug,
    proposalToken,
    length,
    preview ? profile.data : undefined,
  ];
  const readable =
    profile.isSuccess && !needsCalendar && (preview || !profile.data.paused);
  const slots = useQuery({
    queryKey: [...readKey, visible?.from, visible?.to, locale],
    enabled: readable,
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
      length?: number;
    }) => {
      if (proposalToken) {
        return unwrap(
          await api.POST("/public/proposal/{token}", {
            params: {
              path: { token: proposalToken },
            },
            body: {
              start: input.start,
              end: input.end,
              consent: { ...PUBLIC_BOOKING_CONSENT, wording: input.wording },
            },
          }),
        );
      }
      const data = unwrap(
        await api.POST("/public/booking/{host_slug}", {
          params: {
            path: { host_slug: input.slug },
            query: { l: input.length },
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
        }),
      );
      if (!data.invitation) throw new Error(t("book.deliveryUnknown"));
      return data.invitation;
    },
    onMutate: fromHere,
    // Replacing: Back to the booked slot would offer to book it again.
    onSuccess: (value, _input, from) => {
      if (value.management_token)
        from.navigateReplacing({
          screen: "book",
          id: `manage-${value.management_token}`,
        });
    },
    onError: () => {
      void client.invalidateQueries({ queryKey: readKey });
      void profile.refetch();
    },
  });
  const days = slots.data ? monthDays(slots.data, zone) : undefined;
  const monthKey = isoMonth(month);
  const day = pickedDay?.startsWith(monthKey)
    ? pickedDay
    : (days?.free.find((free) => free.startsWith(monthKey)) ?? "");
  const today = dayInZone(now, zone);
  const { dayRead, dayTimes } = useDayTimes({
    day,
    days,
    slots,
    readKey,
    readable,
    read: (window) => readMonth(read, window),
    window: (late) => dayWindow(late, zone, now),
  });
  const pickDay = (next: ISODay) => {
    setPickedDay(next);
    setSelected(null);
    if (!next.startsWith(monthKey)) setMonth(monthOf(next));
  };
  const publicUnavailable = !preview && profile.data?.paused !== false;
  useBookingTitle(preview ? null : guestTitle(t, profile.data), profile.data);
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <QueryGate
          pendingLabel={t("common.loading")}
          query={profile}
          gone={<GoneNotice messageKey="book.linkGone" />}
        >
          {(host) =>
            publicUnavailable ? (
              <GuestUnavailable host={host} zone={zone} />
            ) : (
              <>
                {preview && <PreviewNotice enabled={host.enabled} />}
                {preview && (
                  <UnreadableCalendarLine
                    calendar={unreadableCalendarOf(dayRead.error)}
                    provider={"provider" in host ? (host.provider ?? "") : ""}
                  />
                )}
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
                        lengths={host.lengths}
                        length={length}
                        onLength={(length) => {
                          pickLength(length);
                          setPickedDay("");
                          setSelected(null);
                        }}
                        selected={selected}
                        zone={zone}
                        onZone={setZone}
                      >
                        {host.proposal && (
                          <>
                            <p>{host.proposal.description}</p>
                            <p className="t-caption">
                              {t("book.personalGuest")}
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
                          refusal={(candidate) => {
                            const why = dayRefusal(
                              candidate,
                              today,
                              monthKey,
                              days,
                            );
                            if (why === "past") return t("scheduling.dayPast");
                            if (why === "full") return t("scheduling.dayFull");
                            return undefined;
                          }}
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
                                  length,
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
                            times={dayTimes}
                            monthKey={monthKey}
                            zone={zone}
                            needsCalendar={needsCalendar}
                            slots={dayRead}
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
      </div>
    </div>
  );
}

// A busy month's read stops after a bounded number of pages; a day past where
// it stopped is still open, and its times are read on their own.
function useDayTimes({
  day,
  days,
  slots,
  readKey,
  readable,
  read,
  window,
}: Readonly<{
  day: ISODay | "";
  days: ReturnType<typeof monthDays> | undefined;
  slots: QueryLike<Availability>;
  readKey: readonly unknown[];
  readable: boolean;
  read: (
    window: Readonly<{ from: string; to: string }>,
  ) => Promise<Availability>;
  window: (day: ISODay) => Readonly<{ from: string; to: string }>;
}>) {
  const lateDay = pastKnown(day, days) ? day : "";
  const lateSlots = useQuery({
    queryKey: [...readKey, "day", lateDay, lateDay && window(lateDay).from],
    enabled: readable && lateDay !== "",
    queryFn: () =>
      lateDay ? read(window(lateDay)) : { slots: [], truncated: false },
  });
  if (!lateDay) return { dayRead: slots, dayTimes: days?.byDay.get(day) ?? [] };
  return { dayRead: lateSlots, dayTimes: lateSlots.data?.slots ?? [] };
}
