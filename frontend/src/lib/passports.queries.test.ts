import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { passportName } from "./passports.queries";

type PassportSummary = components["schemas"]["PassportSummary"];

const minted: PassportSummary = {
  id: "5d1c7a40-2b8e-4f3a-9c61-0e7f2a9b3d18",
  label: "Marcus's Claude",
  scopes: [],
  created_at: "2026-07-01T10:00:00Z",
};

const connection = {
  client_id: "dcr_7f3a91c2",
  client_name: "Claude",
  connected_at: "2026-07-01T10:00:00Z",
  renewable: true,
};

const connected: PassportSummary = {
  ...minted,
  label: "oauth:dcr_7f3a91c2",
  connection,
};

describe("passportName", () => {
  it("names a minted passport by the label its human typed", () => {
    expect(passportName(minted)).toBe("Marcus's Claude");
  });

  it("names a connection by the client the human approved", () => {
    expect(passportName(connected)).toBe("Claude");
  });

  // The server answers the raw client id when the registration is gone, and
  // the connection's label carries the same id.
  it("names no connection by its client id", () => {
    const orphaned: PassportSummary = {
      ...connected,
      connection: { ...connection, client_name: "dcr_7f3a91c2" },
    };
    expect(passportName(orphaned)).toBeUndefined();
  });

  it("treats a blank label as no name", () => {
    expect(passportName({ ...minted, label: "  " })).toBeUndefined();
  });
});
