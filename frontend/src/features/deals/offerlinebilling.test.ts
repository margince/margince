// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { lineBillingBody } from "./offerlinebilling";

// What the add-line form sends about repeating, and what it deliberately
// leaves out.
//
// The difference from the product form matters: on a CREATE an unstated
// classification must stay ABSENT rather than becoming an explicit null,
// because that is what lets a line copy its product's classification. Sending
// nulls would overwrite the product's answer with "nobody said".

const empty = {
  billingModel: "",
  billingIntervalMonths: "",
  intervalCount: "",
};

describe("lineBillingBody", () => {
  it("sends nothing at all when nobody classified the line", () => {
    // Absent, not null: a line naming a product takes that product's
    // classification, and an explicit null would erase it.
    expect(lineBillingBody(empty)).toEqual({});
  });

  it("sends a one-off line with neither cadence nor term", () => {
    expect(lineBillingBody({ ...empty, billingModel: "one_time" })).toEqual({
      billing_model: "one_time",
    });
  });

  it("sends a recurring line with its cadence and committed periods", () => {
    expect(
      lineBillingBody({
        billingModel: "recurring",
        billingIntervalMonths: "3",
        intervalCount: "4",
      }),
    ).toEqual({
      billing_model: "recurring",
      billing_interval_months: 3,
      interval_count: 4,
    });
  });

  it("leaves an unsettled term absent rather than guessing one", () => {
    // Drafting a recurring line before the term is agreed is allowed. Sending
    // the offer is what refuses it, and it says which line and what is missing.
    expect(
      lineBillingBody({
        billingModel: "recurring",
        billingIntervalMonths: "1",
        intervalCount: "",
      }),
    ).toEqual({ billing_model: "recurring", billing_interval_months: 1 });
  });

  it("refuses a term of zero or fewer periods", () => {
    for (const count of ["0", "-3"]) {
      expect(
        lineBillingBody({
          billingModel: "recurring",
          billingIntervalMonths: "1",
          intervalCount: count,
        }),
      ).toEqual({ billing_model: "recurring", billing_interval_months: 1 });
    }
  });

  it("refuses a cadence the record does not bill on", () => {
    expect(
      lineBillingBody({
        billingModel: "recurring",
        billingIntervalMonths: "4",
        intervalCount: "2",
      }),
    ).toEqual({ billing_model: "recurring", interval_count: 2 });
  });
});
