import { BookingGuestScreen } from "./booking-guest";
import { BookingInviteScreen } from "./booking-invite";
import { BookingMeetingScreen } from "./booking-meeting";
import { BookingProfileScreen } from "./booking-profile";
import "./book.css";

export { PUBLIC_BOOKING_CONSENT } from "./booking-guest";

export function BookingScreen({ hostSlug }: Readonly<{ hostSlug?: string }>) {
  if (!hostSlug) return <BookingProfileScreen />;
  if (hostSlug === "contact") return <BookingInviteScreen />;
  if (hostSlug.startsWith("contact-"))
    return <BookingInviteScreen contactId={hostSlug.slice(8)} />;
  if (hostSlug.startsWith("meeting-"))
    return <BookingMeetingScreen id={hostSlug.slice(8)} />;
  if (hostSlug.startsWith("proposal-"))
    return <BookingGuestScreen hostSlug="" proposalToken={hostSlug.slice(9)} />;
  if (hostSlug.startsWith("manage-"))
    return <BookingMeetingScreen token={hostSlug.slice(7)} />;
  return <BookingGuestScreen hostSlug={hostSlug} />;
}
