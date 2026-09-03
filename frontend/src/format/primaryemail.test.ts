import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { primaryEmail } from "./primaryemail";

// Which address a message goes to.
//
// The cases here are the shared table the SERVER's picker is held to as well: a
// case added on one side and not the other is the drift
// backend/gates/frontendprimaryemail_test.go exists to catch. Written to be
// read from both languages — one list in, one address out.

type PersonEmail = components["schemas"]["PersonEmail"];

function address(over: Partial<PersonEmail> & { email: string }): PersonEmail {
  return {
    id: `e-${over.email}`,
    email_type: "work",
    is_primary: false,
    position: 0,
    source: "manual",
    captured_by: "human:u-1",
    ...over,
  };
}

describe("the address a contact is written to", () => {
  it("takes the one marked primary", () => {
    const emails = [
      address({ email: "old@buyer.test" }),
      address({ email: "anna@buyer.test", is_primary: true }),
    ];
    expect(primaryEmail(emails)).toBe("anna@buyer.test");
  });

  // An unmarked address is still reachable. A contact with exactly one address
  // and no flag set is the ordinary case, and refusing to write to them would
  // read the flag as permission when it only ranks.
  it("takes the first live one when nothing is marked", () => {
    const emails = [
      address({ email: "anna@buyer.test" }),
      address({ email: "anna.weber@buyer.test" }),
    ];
    expect(primaryEmail(emails)).toBe("anna@buyer.test");
  });

  // The one wrong answer here. Somebody retired that address: mail to it either
  // bounces or reaches a person who asked us to stop using it.
  it("skips an archived address even when it is first", () => {
    const emails = [
      address({
        email: "left@buyer.test",
        archived_at: "2026-01-01T00:00:00Z",
      }),
      address({ email: "anna@buyer.test" }),
    ];
    expect(primaryEmail(emails)).toBe("anna@buyer.test");
  });

  // Archived outranks primary, because retirement is a decision about the
  // address itself and the flag is only a ranking among live ones.
  it("skips an archived address even when it is marked primary", () => {
    const emails = [
      address({
        email: "left@buyer.test",
        is_primary: true,
        archived_at: "2026-01-01T00:00:00Z",
      }),
      address({ email: "anna@buyer.test" }),
    ];
    expect(primaryEmail(emails)).toBe("anna@buyer.test");
  });

  it("answers nothing when every address is archived", () => {
    const emails = [
      address({
        email: "left@buyer.test",
        archived_at: "2026-01-01T00:00:00Z",
      }),
    ];
    expect(primaryEmail(emails)).toBeUndefined();
  });

  it("answers nothing for a contact with no address at all", () => {
    expect(primaryEmail([])).toBeUndefined();
    expect(primaryEmail(undefined)).toBeUndefined();
  });
});
