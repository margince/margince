import { expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  licensePostureOf,
  TRIAL_WARNING_DAYS,
  trialStandingOf,
} from "./license-posture";

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

const DAY = 86_400_000;
const ENDS = "2026-11-16T09:30:00Z";

function trial(ended: boolean): NonNullable<LicenseEntitlement["trial"]> {
  return { started_at: "2026-11-02T09:30:00Z", ends_at: ENDS, ended };
}

it("reads an unlicensed installation on its trial as trial, not as none", () => {
  const onTrial = { ...entitlement("absent"), trial: trial(false) };
  expect(licensePostureOf(onTrial)).toBe("trial");
  expect(licensePostureOf(entitlement("absent"))).toBe("none");
});

it("counts the days left in whole days, rounding the last hours up to one", () => {
  const end = Date.parse(ENDS);
  expect(trialStandingOf(trial(false), end - 10 * DAY).daysLeft).toBe(10);
  expect(trialStandingOf(trial(false), end - 9.5 * DAY).daysLeft).toBe(10);
  expect(trialStandingOf(trial(false), end - 60_000).daysLeft).toBe(1);
});

it("turns to a warning in the last week and holds the server's word on the end", () => {
  const end = Date.parse(ENDS);
  expect(trialStandingOf(trial(false), end - 8 * DAY).closing).toBe(false);
  expect(
    trialStandingOf(trial(false), end - TRIAL_WARNING_DAYS * DAY).closing,
  ).toBe(true);
  // A reader whose clock is a day behind still sees the lock the server holds.
  const ended = trialStandingOf(trial(true), end - DAY);
  expect(ended).toMatchObject({ ended: true, daysLeft: 0, closing: true });
});
