import { expect, it } from "vitest";
import { MARGINCE_TEAM_ADDRESS, trialMailtoHref } from "./trialcontact";

it("addresses the team and encodes the email so a mail app reads it back whole", () => {
  const href = trialMailtoHref({
    subject: "Kessler & Söhne: a Margince license",
    body: "Hello Margince team,\n\nSeats in use: 12 + 3 read seats",
  });
  expect(href.startsWith(`mailto:${MARGINCE_TEAM_ADDRESS}?`)).toBe(true);
  // RFC 6068 spaces are %20; a `+` in the text stays a plus sign.
  expect(href).not.toMatch(/subject=[^&]*\+/);
  const query = new URL(href).searchParams;
  expect(query.get("subject")).toBe("Kessler & Söhne: a Margince license");
  expect(query.get("body")).toBe(
    "Hello Margince team,\n\nSeats in use: 12 + 3 read seats",
  );
});
