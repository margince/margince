// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import type { ContractDraft } from "./contractform";
import { contractTermsBody } from "./contracttermsbody";

// What the contract form sends when an agreement carries recurring value.
//
// The case that matters is an agreement priced ONLY on its recurring value.
// Before the money pairing was rewritten that row was illegal, and a form that
// still tied the currency to the total value would send a stranded ARR and be
// refused for a reason no reader could act on.

const draft = (over: Partial<ContractDraft>): ContractDraft => ({
  title: "Support agreement",
  contractNumber: "",
  valueMinor: 0,
  arrMinor: 0,
  currency: "EUR",
  valueBasis: "total",
  startsOn: "",
  endsOn: "",
  renewalOn: "",
  noticePeriodDays: "",
  paymentTermDays: "",
  signedOn: "",
  ...over,
});

describe("contractTermsBody recurring value", () => {
  it("sends the currency for an agreement priced only on its ARR", () => {
    const body = contractTermsBody(draft({ arrMinor: 1_200_000 }));
    expect(body.arr_minor).toBe(1_200_000);
    expect(body.currency).toBe("EUR");
    // Nothing invents a total for an agreement nobody priced that way.
    expect(body.value_minor).toBeUndefined();
  });

  it("sends both figures under one currency", () => {
    const body = contractTermsBody(
      draft({ valueMinor: 500_000, arrMinor: 1_200_000 }),
    );
    expect(body.value_minor).toBe(500_000);
    expect(body.arr_minor).toBe(1_200_000);
    expect(body.currency).toBe("EUR");
  });

  it("omits the currency when neither figure was priced", () => {
    const body = contractTermsBody(draft({}));
    expect(body.currency).toBeUndefined();
    expect(body.value_minor).toBeUndefined();
    expect(body.arr_minor).toBeUndefined();
  });

  it("sends a figure without its currency rather than inventing one", () => {
    // The installation read that supplies the code can land after the reader
    // has typed. The half-pair goes out for the server to refuse in the open,
    // which is visible, rather than being saved under a guessed unit.
    const body = contractTermsBody(draft({ arrMinor: 50_000, currency: "" }));
    expect(body.arr_minor).toBe(50_000);
    expect(body.currency).toBeUndefined();
  });
});
