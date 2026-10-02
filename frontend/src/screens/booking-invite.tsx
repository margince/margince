import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Breadcrumb } from "../design-system/breadcrumb";
import { ChoiceList } from "../design-system/choicelist";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import {
  BookingBlocked,
  type BookingMode,
  BookingPicker,
  type BookingSlot,
} from "./booking-picker";
import { type BookingEdit, BookingReview } from "./booking-review";
import { throwProblem } from "./common";
import { useSchedulingProfile } from "./scheduling-profile-query";
import { useWorkingHours } from "./working-hours";

type Contact = components["schemas"]["Contact"];

type Edits = Readonly<{
  contact?: { id: string; name: string };
  attendee?: string;
  subject?: string;
  location?: string;
  description?: string;
  video?: boolean;
  duration?: number;
}>;

export function BookingInviteScreen({
  contactId,
}: Readonly<{ contactId?: string }>) {
  const t = useT();
  const [zone, setZone] = useState(viewerZone);
  const [mode, setMode] = useState<BookingMode>("propose");
  const [picks, setPicks] = useState<BookingSlot[]>([]);
  const [edits, setEdits] = useState<Edits>({});
  const [from, setFrom] = useState(() => new Date().toISOString());
  const [searchAhead, setSearchAhead] = useState(false);
  const hours = useWorkingHours(true);
  const profile = useSchedulingProfile(true);
  const selectedId = edits.contact?.id ?? contactId ?? "";
  const contact = useQuery({
    queryKey: ["booking-contact", selectedId],
    enabled: !!selectedId,
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}", {
        params: { path: { id: selectedId } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const saved = profile.data;
  const provider = saved?.provider ?? "";
  const { ready, connections } = useBookingCalendar(provider, false);
  const settled = !profile.isPending && !connections.isPending;
  const name = contact.data?.full_name;
  const draft = bookingDraft(edits, contact.data, saved, selectedId, t);
  const edit = (change: BookingEdit) =>
    setEdits((current) => ({
      ...current,
      ...change,
      ...(change.contact ? { attendee: undefined } : {}),
    }));
  const pick = (slot: BookingSlot) =>
    setPicks((current) => togglePick(current, slot, mode));
  const back = () =>
    navigate(
      contactId
        ? { screen: "contacts", id: contactId, id2: "meetings" }
        : { screen: "home" },
    );
  return (
    <div className="book-page">
      <BookingHead
        contactId={selectedId || undefined}
        name={name}
        onClose={back}
      />
      <ErrorLine error={profile.error} />
      <ChoiceList
        legend={t("scheduling.method")}
        hideLegend
        layout="cards"
        value={mode}
        onChange={(next) => {
          setMode(next);
          if (next === "invite") setPicks((current) => current.slice(0, 1));
        }}
        choices={[
          {
            value: "propose",
            label: t("scheduling.propose"),
            description: t("scheduling.proposeHelp"),
          },
          {
            value: "invite",
            label: t("scheduling.invite"),
            description: t("scheduling.inviteHelp"),
          },
          {
            value: "link",
            label: t("scheduling.sharePersonal"),
            description: t("scheduling.linkHelp"),
          },
        ]}
      />
      <div className="book-compose">
        {/* Every mode keeps the week on screen, a personal link as the times
            the guest will choose from, so changing mode never reflows the page. */}
        {settled && !ready ? (
          <BookingBlocked />
        ) : (
          <BookingPicker
            mode={mode}
            configured={settled && ready}
            profile={saved}
            hours={hours}
            zone={zone}
            onZone={setZone}
            duration={draft.duration}
            onDuration={(duration) => {
              setEdits((current) => ({ ...current, duration }));
              setPicks([]);
            }}
            from={from}
            onFrom={setFrom}
            searchAhead={searchAhead}
            onSearchAhead={setSearchAhead}
            picks={picks}
            onPick={pick}
          />
        )}
        <BookingReview
          mode={mode}
          draft={draft}
          onEdit={edit}
          picks={picks}
          onRemovePick={(slot) =>
            setPicks((current) => current.filter((p) => p.start !== slot.start))
          }
          configured={settled && ready}
          zone={zone}
          searchContacts={searchBookingContacts}
        />
      </div>
    </div>
  );
}

// A proposal offers up to three times and an invite exactly one, so a pick
// toggles within the mode's bound rather than silently growing past it.
export function togglePick(
  current: readonly BookingSlot[],
  slot: BookingSlot,
  mode: BookingMode,
): BookingSlot[] {
  if (current.some((option) => option.start === slot.start))
    return current.filter((option) => option.start !== slot.start);
  if (mode === "invite") return [slot];
  return current.length < 3 ? [...current, slot] : [...current];
}

function bookingDraft(
  edits: Edits,
  contact: Contact | undefined,
  saved: components["schemas"]["SchedulingProfile"] | undefined,
  contactId: string,
  t: ReturnType<typeof useT>,
) {
  return {
    contactId,
    contactName: edits.contact?.name ?? contact?.full_name,
    attendee: edits.attendee ?? bookingRecipient(contact),
    subject: edits.subject ?? saved?.title ?? t("book.subject"),
    location: edits.location ?? saved?.location ?? "",
    description: edits.description ?? "",
    duration: edits.duration ?? saved?.duration_minutes ?? 30,
    video: edits.video ?? saved?.video_call ?? true,
    provider: saved?.provider ?? "",
  };
}

function BookingHead({
  contactId,
  name,
  onClose,
}: Readonly<{ contactId?: string; name?: string; onClose: () => void }>) {
  const t = useT();
  return (
    <header className="book-heading">
      <div className="book-form">
        {contactId && name && (
          <Breadcrumb
            label={t("scheduling.new")}
            items={[
              { label: t("nav.contacts"), href: "#/contacts" },
              { label: name, href: `#/contacts/${contactId}/meetings` },
              { label: t("scheduling.new") },
            ]}
          />
        )}
        <Heading as="h1" size="large">
          {name ? t("scheduling.bookWith", { name }) : t("scheduling.new")}
        </Heading>
      </div>
      <IconAction
        label={t("common.close")}
        icon={<X aria-hidden />}
        onClick={onClose}
      />
    </header>
  );
}

function bookingRecipient(contact?: Contact) {
  const addresses = contact?.emails ?? [];
  return (
    contact?.primary_email ??
    addresses.find((address) => address.is_primary)?.email ??
    (addresses.length === 1 ? addresses[0].email : "")
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
