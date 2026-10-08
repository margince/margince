// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en, type MessageKey } from "../i18n/en";
import { captureValueMessage } from "./capturevalue";
import { ProblemError } from "./common";

const t = (key: MessageKey) => en[key];

function refusal(code: string) {
  return new ProblemError({
    code: "validation_error",
    status: 422,
    detail: "give a bare domain, no address, scheme or path",
    details: { errors: [{ field: "value", code, message: "server words" }] },
  });
}

describe("captureValueMessage", () => {
  it("answers a refused domain with the catalog's sentence for a domain", () => {
    expect(captureValueMessage(refusal("invalid_domain"), "domain", t)).toBe(
      en["captureValue.refusedDomain"],
    );
  });

  it("answers a refused address with the catalog's sentence for an address", () => {
    expect(
      captureValueMessage(refusal("invalid_exclusion"), "address", t),
    ).toBe(en["captureValue.refusedAddress"]);
  });

  // A folder rule is picked from a list, so its refusal has no sentence of its own.
  it("leaves a refused folder rule to the shared problem reading", () => {
    expect(
      captureValueMessage(refusal("invalid_exclusion"), "container", t),
    ).not.toBe(en["captureValue.refusedAddress"]);
  });

  it("leaves a failure that is not a refused value to the shared reading", () => {
    expect(captureValueMessage(refusal("duplicate"), "address", t)).not.toBe(
      en["captureValue.refusedAddress"],
    );
  });
});
