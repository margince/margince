import { useBookingRefused } from "../../lib/scheduling-profile-query";
import { useMe } from "../../lib/session";
import { BookingGuestScreen } from "./booking-guest";
import { BookingInviteScreen } from "./booking-invite";
import { BookingMeetingScreen } from "./booking-meeting";
import { BookingProfileScreen } from "./booking-profile";
import "./book.css";

export { PUBLIC_BOOKING_CONSENT } from "./booking-guest";

export function BookingScreen({ hostSlug }: Readonly<{ hostSlug?: string }>) {
  if (!hostSlug) return <BookingProfileScreen />;
  if (hostSlug === "preview") return <BookingPreview />;
  if (hostSlug === "contact") return <BookingInviteScreen />;
  if (hostSlug.startsWith("contact-"))
    return <BookingInviteScreen key={hostSlug} contactId={hostSlug.slice(8)} />;
  if (hostSlug.startsWith("meeting-"))
    return <BookingMeetingScreen id={hostSlug.slice(8)} />;
  if (hostSlug.startsWith("proposal-"))
    return <BookingGuestScreen hostSlug="" proposalToken={hostSlug.slice(9)} />;
  if (hostSlug.startsWith("manage-"))
    return <BookingMeetingScreen token={hostSlug.slice(7)} />;
  return <BookingGuestScreen hostSlug={hostSlug} />;
}

// The host's own preview reads their booking profile, which only a seat that
// may book has. A seat that may not gets the link page's own refusal, and
// guests never pass here, so the public page never asks /me.
function BookingPreview() {
  const answered = useMe().data?.authorization !== undefined;
  const refused = useBookingRefused();
  if (!answered) return null;
  if (refused) return <BookingProfileScreen />;
  return <BookingGuestScreen hostSlug="" preview />;
}
