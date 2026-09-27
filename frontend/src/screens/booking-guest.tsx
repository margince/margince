import { useMutation, useQuery } from "@tanstack/react-query";
import { CalendarDays, Clock, MapPin } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { navigate } from "../app/router";
import {
  Button,
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { CompanyLogo } from "../design-system/companylogo";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { MeetingSlots } from "../design-system/meetingslots";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { dayInZone, startOfDayInZone, viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { BookingFooter, BookingZone, useBookingIntent } from "./booking-common";

import { throwBookingProblem } from "./booking-errors";
import { QueryGate, throwProblem } from "./common";

export const PUBLIC_BOOKING_CONSENT = { policy_version: "2026-07" };
export function BookingGuestScreen({
  hostSlug,
  proposalToken,
  preview = false,
}: Readonly<{ hostSlug: string; proposalToken?: string; preview?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const [zone, setZone] = useState(viewerZone);
  const intent = useBookingIntent();
  const [from, setFrom] = useState(() => new Date().toISOString());
  const [selected, setSelected] = useState<{
    start: string;
    end: string;
  } | null>(null);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [topic, setTopic] = useState("");
  const [consent, setConsent] = useState(false);
  const profile = useQuery({
    queryKey: preview
      ? ["scheduling-profile"]
      : ["public-booking-profile", hostSlug, proposalToken],
    select: (value) => ({
      ...value,
      host_name: value.host_name ?? "",
      company_name: value.company_name ?? "",
      proposal: "proposal" in value ? value.proposal : null,
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
  const slots = useQuery({
    queryKey: [
      preview ? "reliable-availability" : "public-booking-slots",
      preview ? "booking-preview" : hostSlug,
      proposalToken,
      from,
      locale,
      preview ? profile.data : undefined,
    ],
    enabled:
      profile.isSuccess && !needsCalendar && (preview || profile.data.enabled),
    queryFn: async () => {
      if (preview) {
        const { data, error } = await api.GET("/availability", {
          params: {
            query: {
              from,
              to: new Date(
                new Date(from).getTime() + 7 * 86400000,
              ).toISOString(),
              reliable: true,
            },
          },
        });
        if (error) throwBookingProblem(error, t);
        return data;
      }
      if (proposalToken) {
        const { data, error } = await api.GET(
          "/public/proposal/{token}/availability",
          {
            params: {
              path: { token: proposalToken },
              query: {
                from,
                to: new Date(
                  new Date(from).getTime() + 7 * 86400000,
                ).toISOString(),
              },
            },
          },
        );
        if (error) throwBookingProblem(error, t);
        return data;
      }

      const { data, error } = await api.GET(
        "/public/booking/{host_slug}/availability",
        {
          params: {
            path: { host_slug: hostSlug },
            query: {
              from,
              to: new Date(
                new Date(from).getTime() + 7 * 86400000,
              ).toISOString(),
            },
          },
        },
      );
      if (error) throwBookingProblem(error, t);
      return data;
    },
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
  const publicUnavailable = !preview && !profile.data?.enabled;
  const previewNotice = t(
    profile.data?.enabled
      ? "scheduling.previewActive"
      : "scheduling.previewPaused",
  );
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <QueryGate pendingLabel={t("common.loading")} query={profile}>
          {(host) =>
            publicUnavailable ? (
              <Panel>
                <PanelBody>
                  {host.proposal?.meeting?.management_token ? (
                    <>
                      <Heading as="h1" size="large">
                        {host.proposal.meeting.subject}
                      </Heading>
                      <p>
                        {formatDateTime(
                          host.proposal.meeting.start,
                          locale,
                          zone,
                        )}
                      </p>
                      <p className="t-caption">
                        {t("scheduling.savedRequest")}
                      </p>
                      <Button
                        variant="primary"
                        onClick={() =>
                          navigate({
                            screen: "book",
                            id: `manage-${host.proposal?.meeting?.management_token}`,
                          })
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
            ) : (
              <>
                {preview && (
                  <Panel title={t("scheduling.preview")} tone="accent">
                    <PanelBody>
                      <p>{previewNotice}</p>
                      <a href="#/settings/meetings">
                        {t("scheduling.openSettings")}
                      </a>
                    </PanelBody>
                  </Panel>
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
                    <div className="book-guest-grid">
                      <section className="book-host">
                        <p className="t-caption">{host.host_name}</p>
                        <Heading as="h1" size="large">
                          {host.title}
                        </Heading>
                        <p>
                          <Clock aria-hidden />
                          {t("co.recent.minutes", {
                            count: formatNumber(host.duration_minutes, locale),
                          })}
                        </p>
                        <p>
                          <MapPin aria-hidden />
                          {host.location}
                        </p>
                        <p>
                          <CalendarDays aria-hidden />
                          {zone}
                        </p>
                      </section>
                      <section className="book-form">
                        <Heading as="h2" size="medium">
                          {t("scheduling.chooseTime")}
                        </Heading>
                        {host.proposal && (
                          <>
                            <p>{host.proposal.description}</p>
                            <p className="t-caption">
                              {t("scheduling.personalGuest")}
                            </p>
                            {host.proposal.options.length > 0 && (
                              <MeetingSlots
                                slots={host.proposal.options.map((slot) => ({
                                  ...slot,
                                  label: formatDateTime(
                                    slot.start,
                                    locale,
                                    zone,
                                  ),
                                }))}
                                selected={selected?.start}
                                onSelect={setSelected}
                                empty={t("scheduling.noTimes")}
                              />
                            )}
                          </>
                        )}
                        <BookingZone value={zone} onChange={setZone} />
                        <Field label={t("scheduling.date")}>
                          {(control) => (
                            <TextInput
                              {...control}
                              type="date"
                              value={dayInZone(new Date(from).getTime(), zone)}
                              onChange={(e) => {
                                if (e.target.value) {
                                  setFrom(
                                    startOfDayInZone(e.target.value, zone),
                                  );
                                  setSelected(null);
                                }
                              }}
                            />
                          )}
                        </Field>
                        {needsCalendar ? (
                          <p className="t-caption">
                            {t("scheduling.previewCalendarSetup")}
                          </p>
                        ) : (
                          <QueryGate
                            pendingLabel={t("common.loading")}
                            query={slots}
                          >
                            {(value) => (
                              <>
                                <MeetingSlots
                                  slots={value.slots.map((slot) => ({
                                    ...slot,
                                    label: formatDateTime(
                                      slot.start,
                                      locale,
                                      zone,
                                    ),
                                  }))}
                                  selected={selected?.start}
                                  onSelect={setSelected}
                                  empty={t("scheduling.noTimes")}
                                />
                                {value.truncated && (
                                  <Button
                                    onClick={() => {
                                      const last = value.slots.at(-1);
                                      if (last) {
                                        setFrom(
                                          new Date(
                                            new Date(last.start).getTime() +
                                              15 * 60000,
                                          ).toISOString(),
                                        );
                                        setSelected(null);
                                      }
                                    }}
                                  >
                                    {t("scheduling.next")}
                                  </Button>
                                )}
                              </>
                            )}
                          </QueryGate>
                        )}
                        <form
                          className="book-form"
                          onSubmit={(e) => {
                            e.preventDefault();
                            if (!preview && selected && consent)
                              book.mutate({
                                slug: hostSlug,
                                name,
                                email,
                                topic,
                                ...selected,
                                wording: t("book.consentWording"),
                              });
                          }}
                        >
                          {!proposalToken && (
                            <>
                              <Field label={t("book.name")}>
                                {(control) => (
                                  <TextInput
                                    {...control}
                                    required
                                    autoComplete="name"
                                    value={name}
                                    onChange={(e) => setName(e.target.value)}
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
                                    value={email}
                                    onChange={(e) => setEmail(e.target.value)}
                                  />
                                )}
                              </Field>
                              <Field label={t("scheduling.guestAgenda")}>
                                {(control) => (
                                  <Textarea
                                    {...control}
                                    value={topic}
                                    onChange={(e) => setTopic(e.target.value)}
                                  />
                                )}
                              </Field>
                            </>
                          )}
                          <Checkbox
                            checked={consent}
                            onChange={(event) =>
                              setConsent(event.target.checked)
                            }
                            label={t("book.consentWording")}
                          />
                          <Button
                            type="submit"
                            variant="primary"
                            disabled={
                              preview || !selected || !consent || book.isPending
                            }
                          >
                            {t("scheduling.book")}
                          </Button>
                          <ErrorLine error={book.error} />
                        </form>
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
