import { expect, it } from "vitest";
import type { components } from "../api/schema";
import { licensePostureOf } from "./license-posture";

type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];
type LicenseHolder = components["schemas"]["LicenseHolder"];

const HOLDER: LicenseHolder = {
  id: "lic-1",
  subject: "acme-prod",
  expiry: "2026-08-01T00:00:00Z",
  in_grace: false,
  renewal_due: false,
};

function entitlement(
  state: LicenseEntitlement["state"],
  license?: LicenseHolder,
): LicenseEntitlement {
  return {
    state,
    ...(license && { license }),
    seats_used: 3,
    over_limit: false,
    checked_at: "2026-08-01T09:00:00Z",
  };
}

it("reads a valid licence past its expiry as grace", () => {
  const graced = { ...HOLDER, in_grace: true, renewal_due: true };
  expect(licensePostureOf(entitlement("valid", graced))).toBe("grace");
});

it("reads a valid licence that is only due for renewal as ok", () => {
  const due = { ...HOLDER, renewal_due: true };
  expect(licensePostureOf(entitlement("valid", due))).toBe("ok");
});

it("reads a refusal as refused even when a licence in grace is attached", () => {
  const graced = { ...HOLDER, in_grace: true, renewal_due: true };
  expect(licensePostureOf(entitlement("rejected", graced))).toBe("refused");
});

it.each<[LicenseEntitlement["state"], string]>([
  ["valid", "ok"],
  ["rejected", "refused"],
  ["absent", "none"],
])("reads %s without a licence holder as %s", (state, posture) => {
  expect(licensePostureOf(entitlement(state))).toBe(posture);
});
