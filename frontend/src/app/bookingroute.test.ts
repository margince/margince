import { describe, expect, it } from "vitest";
import { isPublicBookingId } from "./bookingroute";

describe("booking session boundary", () => {
  it.each([undefined, "contact", "contact-record", "meeting-invitation"])(
    "requires a session for host controls at %s",
    (id) => expect(isPublicBookingId(id)).toBe(false),
  );
  it.each(["host-slug", "proposal-private", "manage-private"])(
    "admits guest booking routes at %s without a session",
    (id) => expect(isPublicBookingId(id)).toBe(true),
  );
});
