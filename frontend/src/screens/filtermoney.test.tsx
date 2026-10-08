// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { FilterBuilder } from "./filterbuilder";
import { moneyText } from "./filtersentence";
import { ValueControl } from "./filtervalue";
import {
  encode,
  type FilterOp,
  type LeafValue,
  type Node,
  newGroup,
  newLeaf,
} from "./segmentpredicate";
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

/** A saved revenue clause, in a vocabulary that names no currency for it. */
function UnpricedClause() {
  const [tree, setTree] = useState<Node>(() =>
    newGroup("and", [newLeaf("revenue", "gt", 125_000)]),
  );
  return (
    <>
      <FilterBuilder
        tree={tree}
        onChange={setTree}
        fields={[
          {
            name: "revenue",
            type: "currency",
            operators: ["gt", "exists"],
            custom: false,
          },
        ]}
      />
      <pre data-testid="wire">{JSON.stringify(encode(tree))}</pre>
    </>
  );
}

describe("a money clause whose currency is unknown", () => {
  it("holds its amount back, says why, and can still be removed", async () => {
    render(
      <StoryProviders>
        <UnpricedClause />
      </StoryProviders>,
    );
    const user = userEvent.setup();
    const amount = screen.getByRole("textbox", { name: en["filters.value"] });
    expect(amount).toBeDisabled();
    expect(amount).toHaveValue("");
    expect(amount).toHaveAccessibleDescription(en["filters.amountUnpriced"]);
    expect(screen.queryByDisplayValue("125000")).toBeNull();
    expect(screen.getByTestId("wire")).toHaveTextContent("125000");

    await user.click(
      screen.getByRole("button", { name: "Remove revenue condition" }),
    );
    expect(screen.getByTestId("wire")).toHaveTextContent(/^\{"and":\[\]\}$/);
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
