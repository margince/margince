/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import type { OfferLineItem, UpdateOfferLineItemRequest } from "./offers";
import { UnitCell } from "./offers.cells";
import { isUnnamedOtherUnit, storedUnitOf, unitChoiceOf } from "./productunit";

afterEach(cleanup);

describe("a stored unit and the picker's two controls", () => {
  it("opens a key as that choice and typed text as Other", () => {
    expect(unitChoiceOf("day")).toEqual({ choice: "day", other: "" });
    expect(unitChoiceOf("Lizenz")).toEqual({
      choice: "other",
      other: "Lizenz",
    });
    expect(unitChoiceOf("")).toEqual({ choice: "", other: "" });
  });

  it("opens a row written as free text as the key its spelling names", () => {
    expect(unitChoiceOf(" Tag ")).toEqual({ choice: "day", other: "" });
    expect(unitChoiceOf("Stunden")).toEqual({ choice: "hour", other: "" });
    expect(unitChoiceOf("unit")).toEqual({ choice: "piece", other: "" });
    expect(unitChoiceOf("Tháng")).toEqual({ choice: "month", other: "" });
  });

  it("calls Other with nothing typed unnamed", () => {
    expect(isUnnamedOtherUnit({ choice: "other", other: "  " })).toBe(true);
    expect(isUnnamedOtherUnit({ choice: "other", other: "Seat" })).toBe(false);
    expect(isUnnamedOtherUnit({ choice: "", other: "" })).toBe(false);
  });

  it("stores the key, or the typed text trimmed when Other is picked", () => {
    expect(storedUnitOf("hour", "ignored")).toBe("hour");
    expect(storedUnitOf("other", "  Seat ")).toBe("Seat");
    expect(storedUnitOf("other", "   ")).toBe("");
  });
});

function line(unit: string): OfferLineItem {
  return {
    id: "l-1",
    position: 1,
    description: "Workshop",
    unit,
    quantity: 1,
    unit_price_minor: 100,
    discount_pct: 0,
    tax_rate: 19,
    line_net_minor: 100,
    line_tax_minor: 19,
    line_total_minor: 119,
    price_grounded: true,
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  };
}

function renderCell(unit: string, saved: UpdateOfferLineItemRequest[]) {
  render(
    <LocaleProvider initial="de">
      <UnitCell line={line(unit)} onSave={(patch) => saved.push(patch)} />
    </LocaleProvider>,
  );
}

async function pick(user: ReturnType<typeof userEvent.setup>, choice: string) {
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Einheit" }),
    choice,
  );
}

describe("UnitCell", () => {
  it("shows the stored key in the reader's language and saves a new pick at once", async () => {
    const saved: UpdateOfferLineItemRequest[] = [];
    const user = userEvent.setup();
    renderCell("day", saved);
    expect(screen.getByRole("combobox", { name: "Einheit" }).textContent).toBe(
      "Tag",
    );
    await pick(user, "Stunde");
    expect(saved).toEqual([{ unit: "hour" }]);
  });

  it("shows a row written as free text translated, as the key it names", () => {
    renderCell("Pauschale", []);
    expect(screen.getByRole("combobox", { name: "Einheit" }).textContent).toBe(
      "Pauschale",
    );
    cleanup();
    renderCell("hours", []);
    expect(screen.getByRole("combobox", { name: "Einheit" }).textContent).toBe(
      "Stunde",
    );
  });

  it("saves a typed unit when its box loses focus, and never a blank one", async () => {
    const saved: UpdateOfferLineItemRequest[] = [];
    const user = userEvent.setup();
    renderCell("day", saved);
    await pick(user, "Andere");
    expect(saved).toEqual([]);
    const typed = screen.getByRole("textbox", { name: "Andere Einheit" });
    await user.type(typed, "Lizenz");
    expect(saved).toEqual([]);
    await user.tab();
    expect(saved).toEqual([{ unit: "Lizenz" }]);
  });
});
