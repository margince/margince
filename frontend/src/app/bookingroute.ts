// A host's controls require a session; guest capabilities and reusable slugs do not.
export function isPublicBookingId(id?: string): boolean {
  return Boolean(
    id &&
      id !== "contact" &&
      !id.startsWith("contact-") &&
      !id.startsWith("meeting-"),
  );
}
