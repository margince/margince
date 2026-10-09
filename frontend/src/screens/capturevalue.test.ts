// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en, type MessageKey } from "../i18n/en";
import { captureValueMessage } from "./capturevalue";
import { ProblemError, problemMessageOf } from "./common";

const t = (key: MessageKey) => en[key];

function refusal(field: string, code: string, message: string) {
  return new ProblemError({
    code: "validation_error",
    status: 422,
    detail: message,
    details: { errors: [{ field, code, message }] },
  });
}

const BARE_DOMAIN = "give a bare domain — no address, scheme or path";
const ONE_ADDRESS =
  "give one email address of at most 320 characters, for example name@example.com";
const PUBLIC_SUFFIX =
  "co.uk is a public suffix, not a company's domain — give the domain you register mail under";

describe("captureValueMessage", () => {
  it("answers a value with no domain shape with the catalog's sentence", () => {
    const err = refusal("domain", "invalid_domain", BARE_DOMAIN);
    expect(captureValueMessage(err, "domain", t)).toBe(
      en["captureValue.refusedDomain"],
    );
  });

  it("reads a domain with no dot as the same shape failure", () => {
    const err = refusal("value", "invalid_exclusion", "acme is not a domain");
    expect(captureValueMessage(err, "domain", t)).toBe(
      en["captureValue.refusedDomain"],
    );
  });

  it("answers a value with no address shape with the catalog's sentence", () => {
    const err = refusal("value", "invalid_exclusion", ONE_ADDRESS);
    expect(captureValueMessage(err, "address", t)).toBe(
      en["captureValue.refusedAddress"],
    );
  });

  it("keeps the server's reason for a public suffix", () => {
    const err = refusal("domain", "invalid_domain", PUBLIC_SUFFIX);
    expect(captureValueMessage(err, "domain", t)).toBe(
      problemMessageOf(err, t),
    );
    expect(problemMessageOf(err, t)).toBe(PUBLIC_SUFFIX);
  });

  it("keeps the server's reason for an IP address", () => {
    const err = refusal(
      "value",
      "invalid_exclusion",
      "10.0.0.1 is an IP address; give the domain your mail is addressed to",
    );
    expect(captureValueMessage(err, "domain", t)).toBe(
      problemMessageOf(err, t),
    );
  });

  // A scope or kind refusal names another field, so the value's copy is wrong.
  it("leaves a refusal of another field to the shared reading", () => {
    const err = refusal("scope", "invalid_exclusion", BARE_DOMAIN);
    expect(captureValueMessage(err, "domain", t)).toBe(
      problemMessageOf(err, t),
    );
  });

  // A folder rule is picked from a list, so its refusal has no sentence of its own.
  it("leaves a refused folder rule to the shared reading", () => {
    const err = refusal("value", "invalid_exclusion", ONE_ADDRESS);
    expect(captureValueMessage(err, "container", t)).toBe(
      problemMessageOf(err, t),
    );
  });

  it("leaves a failure that is not a refused value to the shared reading", () => {
    const err = refusal("value", "duplicate", ONE_ADDRESS);
    expect(captureValueMessage(err, "address", t)).toBe(
      problemMessageOf(err, t),
    );
  });
});
