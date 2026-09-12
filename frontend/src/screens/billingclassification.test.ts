// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { billingOf, billingPatchOf } from "./billingclassification";

// What the two editors send when somebody classifies a price, and — the case
// that matters most — what they send when somebody does not.
//
// An empty selection is a real answer: nobody has said whether this price
// repeats. Every product and line written before the field existed carries it,
// so a form that quietly turned it into "one-off" would assert a
// classification across the whole catalogue that nobody made.

describe("billingOf", () => {
  it("sends nulls on a create when nobody has classified the price", () => {
    // On a create a null is unambiguous: the product is born unclassified.
    expect(billingOf({})).toEqual({
      billing_model: null,
      billing_interval_months: null,
    });
    expect(billingOf({ billing_model: "" })).toEqual({
      billing_model: null,
      billing_interval_months: null,
    });
  });

  it("sends a one-off price with no cadence", () => {
    expect(billingOf({ billing_model: "one_time" })).toEqual({
      billing_model: "one_time",
      billing_interval_months: null,
    });
  });

  it("drops a stale cadence when the price becomes one-off", () => {
    // The two controls are independent, so a reader who picks "recurring",
    // sets a cadence and then switches to "one-off" leaves the second control
    // showing a value. Sending it would be refused, and refused over something
    // the reader cannot see they did.
    expect(
      billingOf({ billing_model: "one_time", billing_interval_months: "3" }),
    ).toEqual({ billing_model: "one_time", billing_interval_months: null });
  });

  it("sends a recurring price with its cadence", () => {
    for (const [months, expected] of [
      ["1", 1],
      ["3", 3],
      ["6", 6],
      ["12", 12],
    ] as const) {
      expect(
        billingOf({
          billing_model: "recurring",
          billing_interval_months: months,
        }),
      ).toEqual({
        billing_model: "recurring",
        billing_interval_months: expected,
      });
    }
  });

  it("sends a recurring price with no cadence as the half-answer it is", () => {
    // Not silently completed with a guess. The server refuses it and names the
    // missing field, which tells the reader what to pick; inventing "monthly"
    // here would record a cadence nobody chose.
    expect(billingOf({ billing_model: "recurring" })).toEqual({
      billing_model: "recurring",
      billing_interval_months: null,
    });
  });

  it("refuses a cadence the schema does not bill on", () => {
    // Four months is not one of the periods the record admits, so it travels
    // as absent rather than as a number the server would reject by constraint.
    expect(
      billingOf({ billing_model: "recurring", billing_interval_months: "4" }),
    ).toEqual({ billing_model: "recurring", billing_interval_months: null });
  });

  it("treats a non-string selection as no answer", () => {
    // The form hands these callbacks Record<string, unknown>, so a value that
    // is not a string is a field nobody filled in rather than a value to coerce.
    expect(billingOf({ billing_model: 3 })).toEqual({
      billing_model: null,
      billing_interval_months: null,
    });
  });

  it("says not specified on a PATCH rather than sending a null", () => {
    // A null arrives at the server as an omitted field, and an omitted field
    // has to keep meaning "leave the classification alone". Without a word of
    // its own, the "Not specified" option would silently do nothing and a
    // product once classified could never be un-classified.
    expect(billingPatchOf({})).toEqual({
      billing_model: "not_specified",
      billing_interval_months: null,
    });
  });

  it("passes a real classification through unchanged on a PATCH", () => {
    expect(
      billingPatchOf({
        billing_model: "recurring",
        billing_interval_months: "6",
      }),
    ).toEqual({ billing_model: "recurring", billing_interval_months: 6 });
  });
});
