import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { MeetingSlots } from "../design-system/meetingslots";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordPicker } from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { formatDate, formatDateTime, formatNumber } from "../format/format";
import { dayInZone, startOfDayInZone, viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { useInviteAvailability } from "./booking-availability";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingBack, BookingZone, useBookingIntent } from "./booking-common";
import { BookingProposal } from "./booking-proposal";
import { BookingSetup } from "./booking-setup";
import { QueryGate, throwProblem } from "./common";
import { useSchedulingProfile } from "./scheduling-profile-query";
import { useWorkingHours } from "./working-hours";

type Request = components["schemas"]["MeetingInvitationRequest"];
export function BookingInviteScreen({
  contactId,
}: Readonly<{ contactId?: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [zone, setZone] = useState(viewerZone);
  const intent = useBookingIntent();
  const [mode, setMode] = useState("propose");
  const [options, setOptions] = useState<{ start: string; end: string }[]>([]);
  const [selectedContactId, setSelectedContactId] = useState(contactId ?? "");
  const [email, setEmail] = useState<string | null>(null);
  const [editedSubject, setSubject] = useState<string | null>(null);
  const [editedLocation, setLocation] = useState<string | null>(null);
  const [description, setDescription] = useState("");
  const [searchAhead, setSearchAhead] = useState(false);
  const hours = useWorkingHours(true);
  const [from, setFrom] = useState(() => new Date().toISOString());
  const [editedDuration, setDuration] = useState<number | null>(null);
  const [selected, setSelected] = useState<{
    start: string;
    end: string;
  } | null>(null);
  const contact = useQuery({
    queryKey: ["booking-contact", selectedContactId],
    enabled: !!selectedContactId,
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}", {
        params: { path: { id: selectedContactId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const profile = useSchedulingProfile(true);
  const defaults = profile.data ?? {
    title: t("book.subject"),
    location: "",
    duration_minutes: 30,
    provider: "",
  };
  const subject = editedSubject ?? defaults.title;
  const location = editedLocation ?? defaults.location;
  const duration = editedDuration ?? defaults.duration_minutes;
  const attendee = email ?? bookingRecipient(contact.data);
  const { ready, connections } = useBookingCalendar(defaults.provider, false);
  const configured = ready;
  const { earliest, latest, outsideHorizon, slots } = useInviteAvailability(
    from,
    duration,
    searchAhead,
    configured,
    profile.data,
    hours.data,
  );
  const send = useMutation({
    mutationFn: async (body: Request) => {
      const { data, error } = await api.POST("/scheduling/invitations", {
        body,
        params: { header: { "Idempotency-Key": intent(body) } },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: (value) =>
      navigate({ screen: "book", id: `meeting-${value.id}` }),
  });
  return (
    <div className="book-page">
      <BookingBack />
      <header className="book-heading">
        <Heading as="h1" size="large">
          {t("scheduling.new")}
        </Heading>
        <Button onClick={() => navigate({ screen: "book" })}>
          {t("scheduling.myLink")}
        </Button>
      </header>
      <InviteSetup
        profile={profile.data}
        pending={profile.isPending}
        connectionsPending={connections.isPending}
        error={profile.error}
        configured={configured}
      />
      <div className="book-mode">
        <Select
          aria-label={t("scheduling.method")}
          value={mode}
          onChange={setMode}
          options={[
            { value: "propose", label: t("scheduling.propose") },
            { value: "invite", label: t("scheduling.invite") },
            { value: "link", label: t("scheduling.sharePersonal") },
          ]}
        />
      </div>
      <div className="book-profile-grid">
        <Panel title={t(bookingModeLabel(mode))}>
          <PanelBody>
            <div className="book-form">
              <RecordPicker
                label={t("scheduling.contact")}
                selected={
                  contact.data
                    ? { id: selectedContactId, name: contact.data.full_name }
                    : null
                }
                onPick={(value) => {
                  setSelectedContactId(value.id);
                  setEmail(null);
                }}
                searchTargets={searchBookingContacts}
              />
              <Field label={t("book.attendee")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="email"
                    value={attendee}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                )}
              </Field>
              <Field label={t("scheduling.subject")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={subject}
                    onChange={(e) => setSubject(e.target.value)}
                  />
                )}
              </Field>
              <Field label={t("scheduling.duration")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={15}
                    max={480}
                    value={duration}
                    onChange={(e) => {
                      setDuration(Number(e.target.value));
                      setOptions([]);
                      setSelected(null);
                    }}
                  />
                )}
              </Field>

              <Field label={t("scheduling.location")}>
                {(control) => (
                  <TextInput
                    {...control}
                    value={location}
                    onChange={(e) => setLocation(e.target.value)}
                  />
                )}
              </Field>
              <Field label={t("scheduling.agenda")}>
                {(control) => (
                  <Textarea
                    {...control}
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                  />
                )}
              </Field>
              {selected && (
                <p>{formatDateTime(selected.start, locale, zone)}</p>
              )}
              {mode === "invite" ? (
                <Button
                  variant="primary"
                  disabled={
                    !configured ||
                    !selectedContactId ||
                    !attendee ||
                    !subject ||
                    !selected ||
                    send.isPending
                  }
                  onClick={() => {
                    if (selected)
                      send.mutate({
                        contact_id: selectedContactId,
                        attendee_email: attendee,
                        subject,
                        location,
                        description,
                        ...selected,
                      });
                  }}
                >
                  {t("scheduling.invite")}
                </Button>
              ) : (
                <BookingProposal
                  available={configured}
                  personalOnly={mode === "link"}
                  zone={zone}
                  request={{
                    contact_id: selectedContactId,
                    attendee_email: attendee,
                    subject,
                    location,
                    description,
                    duration_minutes: duration,
                    options: mode === "link" ? [] : options,
                  }}
                />
              )}
              <ErrorLine error={send.error} />
            </div>
          </PanelBody>
        </Panel>
        {mode !== "link" && (
          <Panel title={t("scheduling.chooseTime")}>
            <PanelBody>
              <div className="book-form">
                <Field label={t("scheduling.date")}>
                  {(control) => (
                    <TextInput
                      {...control}
                      type="date"
                      min={dayInZone(earliest, zone)}
                      max={dayInZone(latest, zone)}
                      value={dayInZone(new Date(from).getTime(), zone)}
                      onChange={(e) => {
                        if (e.target.value) {
                          setFrom(startOfDayInZone(e.target.value, zone));
                          setSearchAhead(false);
                          setSelected(null);
                          setOptions([]);
                        }
                      }}
                    />
                  )}
                </Field>
                <p className="t-caption">
                  {t("scheduling.bookingUntil", {
                    date: formatDate(
                      new Date(latest).toISOString(),
                      locale,
                      zone,
                    ),
                  })}
                </p>
                {mode === "propose" && (
                  <p className="t-caption">
                    {plural("scheduling.selectOptions", options.length, {
                      count: formatNumber(options.length, locale),
                    })}
                  </p>
                )}
                <BookingZone value={zone} onChange={setZone} />
                <AvailabilityNotice
                  hours={hours}
                  configured={configured}
                  outsideHorizon={outsideHorizon}
                />
                {configured && !outsideHorizon && (
                  <QueryGate pendingLabel={t("common.loading")} query={slots}>
                    {(value) => (
                      <>
                        <MeetingSlots
                          slots={value.slots.map((slot) => ({
                            ...slot,
                            label: formatDateTime(slot.start, locale, zone),
                          }))}
                          selected={selected?.start}
                          selectedMany={
                            mode === "propose"
                              ? options.map((slot) => slot.start)
                              : undefined
                          }
                          onSelect={(slot) => {
                            if (mode === "propose")
                              setOptions((current) =>
                                toggleProposalOption(current, slot),
                              );
                            else setSelected(slot);
                          }}
                          empty={t(
                            searchAhead
                              ? "scheduling.noTimesHorizon"
                              : "scheduling.noTimes",
                          )}
                        />
                        <EmptyAvailability
                          empty={value.slots.length === 0}
                          searched={searchAhead}
                          onSearch={() => setSearchAhead(true)}
                        />
                        {value.truncated && (
                          <Button
                            onClick={() => {
                              const last = value.slots.at(-1);
                              if (last) {
                                setFrom(
                                  new Date(
                                    new Date(last.start).getTime() + 15 * 60000,
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
              </div>
            </PanelBody>
          </Panel>
        )}
      </div>
    </div>
  );
}

function bookingRecipient(contact?: components["schemas"]["Contact"]) {
  const addresses = contact?.emails ?? [];
  return (
    contact?.primary_email ??
    addresses.find((address) => address.is_primary)?.email ??
    (addresses.length === 1 ? addresses[0].email : "")
  );
}

function toggleProposalOption(
  current: { start: string; end: string }[],
  slot: { start: string; end: string },
) {
  if (current.some((option) => option.start === slot.start))
    return current.filter((option) => option.start !== slot.start);
  return current.length < 3 ? [...current, slot] : current;
}

function bookingModeLabel(mode: string) {
  if (mode === "propose") return "scheduling.propose";
  return mode === "link" ? "scheduling.sharePersonal" : "scheduling.invite";
}

function InviteSetup({
  profile,
  pending,
  error,
  configured,
  connectionsPending,
}: Readonly<{
  profile?: components["schemas"]["SchedulingProfile"];
  pending: boolean;
  error: unknown;
  configured: boolean;
  connectionsPending: boolean;
}>) {
  const t = useT();
  return (
    <>
      {(pending || connectionsPending) && <p>{t("common.loading")}</p>}
      <ErrorLine error={error} />
      {profile && !pending && !connectionsPending && !configured && (
        <BookingSetup />
      )}
    </>
  );
}

function EmptyAvailability({
  empty,
  searched,
  onSearch,
}: Readonly<{ empty: boolean; searched: boolean; onSearch: () => void }>) {
  const t = useT();
  if (!empty) return null;
  return (
    <div className="book-form">
      <p className="t-caption">{t("scheduling.noTimesHelp")}</p>
      <p className="t-caption">{t("scheduling.allDayBlocks")}</p>
      {!searched && (
        <Button onClick={onSearch}>{t("scheduling.findNext")}</Button>
      )}
      <a href="#/settings/meetings" target="_blank" rel="noreferrer">
        {t("scheduling.openSettings")}
      </a>
    </div>
  );
}

async function searchBookingContacts(q: string) {
  const { data, error } = await api.GET("/search", {
    params: { query: { q, limit: 10 } },
  });
  if (error) throwProblem(error);
  return data.data
    .filter((hit) => hit.type === "contact")
    .map((hit) => ({ id: hit.id, name: hit.title ?? hit.id }));
}

function AvailabilityNotice({
  hours,
  configured,
  outsideHorizon,
}: Readonly<{
  hours: ReturnType<typeof useWorkingHours>;
  configured: boolean;
  outsideHorizon: boolean;
}>) {
  const t = useT();
  return (
    <>
      {hours.data?.working_hours && (
        <p className="t-caption">
          {t("scheduling.effectiveHours", {
            start: hours.data.working_hours.start_time,
            end: hours.data.working_hours.end_time,
            zone: hours.data.working_hours.timezone,
          })}
        </p>
      )}
      <ErrorLine error={hours.error} />
      {!configured && <p>{t("scheduling.finishSetup")}</p>}
      {configured && outsideHorizon && (
        <div className="book-form">
          <p>{t("scheduling.outsideHorizon")}</p>
          <a href="#/settings/meetings" target="_blank" rel="noreferrer">
            {t("scheduling.openSettings")}
          </a>
        </div>
      )}
    </>
  );
}
