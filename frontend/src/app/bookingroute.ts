import { BOOKING_HOST_IDS } from "../lib/bookingaddress";

// A host's controls require a session; guest capabilities and reusable slugs do not.
export function isPublicBookingId(id?: string): boolean {
  return Boolean(
    id &&
      !BOOKING_HOST_IDS.includes(id) &&
      !id.startsWith("contact-") &&
      !id.startsWith("meeting-"),
  );
}
