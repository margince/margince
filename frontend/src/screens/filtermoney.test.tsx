// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { ValueControl } from "./filtervalue";
import { moneyText } from "./listwhy";
import type { FilterOp, LeafValue } from "./segmentpredicate";
import { StoryProviders } from "./story-utils";

afterEach(cleanup);

function AmountClause({
  currency,
  op,
  seen,
}: Readonly<{ currency: string; op: FilterOp; seen: LeafValue[] }>) {
  const [value, setValue] = useState<LeafValue>(op === "in" ? [] : "");
  return (
    <ValueControl
      type="currency"
      currency={currency}
      references={undefined}
      options={undefined}
      op={op}
      value={value}
      label="Amount"
      onChange={(next) => {
        seen.push(next);
        setValue(next);
      }}
    />
  );
}

function typeAmount(currency: string, typed: string): LeafValue[] {
  const seen: LeafValue[] = [];
  render(
    <StoryProviders>
      <AmountClause currency={currency} op="gte" seen={seen} />
    </StoryProviders>,
  );
  fireEvent.change(screen.getByLabelText(`Amount (${currency})`), {
    target: { value: typed },
  });
  return seen;
}

describe("a money clause", () => {
  it("sends a euro amount typed in euros as cents", () => {
    expect(typeAmount("EUR", "10000").at(-1)).toBe(1_000_000);
  });

  it("takes decimals in major units", () => {
    expect(typeAmount("EUR", "100.50").at(-1)).toBe(10_050);
  });

  it("sends a yen amount unscaled, yen having no minor unit", () => {
    expect(typeAmount("JPY", "10000").at(-1)).toBe(10_000);
  });

  it("takes the amount back when the box is emptied, and stays empty", () => {
    const seen = typeAmount("EUR", "100");
    const box = screen.getByLabelText("Amount (EUR)");
    fireEvent.change(box, { target: { value: "" } });
    expect(seen.at(-1)).toBe("");
    fireEvent.blur(box);
    expect(box).toHaveValue(null);
  });

  it("shows the currency it is typed in", () => {
    typeAmount("EUR", "1");
    expect(screen.getByText("EUR")).toBeInTheDocument();
  });

  it("scales every amount of a list", () => {
    const seen: LeafValue[] = [];
    render(
      <StoryProviders>
        <AmountClause currency="EUR" op="in" seen={seen} />
      </StoryProviders>,
    );
    const box = screen.getByLabelText("Amount (EUR)");
    fireEvent.change(box, { target: { value: "250" } });
    fireEvent.keyDown(box, { key: "Enter" });
    expect(seen.at(-1)).toEqual([25_000]);
  });
});

describe("a money clause explained", () => {
  it("reads minor units back as money in the field's currency", () => {
    expect(moneyText(1_000_000, "EUR", "en")).toBe("€10,000.00");
    expect(moneyText("1000000", "EUR", "en")).toBe("€10,000.00");
    expect(moneyText(10_000, "JPY", "en")).toBe("JP¥10,000");
    expect(moneyText("not money", "EUR", "en")).toBeUndefined();
  });
});
