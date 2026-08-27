/** @vitest-environment jsdom */

import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { HistoryFieldDiff } from "./historyfielddiff";

function show(node: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

afterEach(cleanup);

describe("HistoryFieldDiff", () => {
  // The audit spine projects every value as a string, including the integer
  // minor-unit count money is stored in. Rendered raw, a deal moving from
  // 2500000 to 4150000 reads as millions beside tiles showing the same figure
  // as €25,000.00 — and a value misread by a hundredfold is the one thing that
  // must not sit next to a button that writes.
  it("scales a minor-unit field by the record's currency", () => {
    show(
      <HistoryFieldDiff
        field="amount_minor"
        oldValue="2500000"
        newValue="4150000"
        values={{ currency: "EUR", locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.getByText(/25,000\.00/)).toBeTruthy();
    expect(screen.getByText(/41,500\.00/)).toBeTruthy();
  });

  // A field with no minor-unit suffix is not money and keeps its own text.
  it("leaves a non-money field alone", () => {
    show(
      <HistoryFieldDiff
        field="title"
        oldValue="Ops Lead"
        newValue="Head of Ops"
        values={{ currency: "EUR", locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.getByText("Ops Lead")).toBeTruthy();
    expect(screen.getByText("Head of Ops")).toBeTruthy();
  });

  // A null side is the created/cleared origin, and the diff draws its own
  // wording for it. Asserted through the rendered text rather than re-spelled
  // here: one owner of that phrasing, and it is FieldDiff.
  it("hands a null side to the diff as null rather than as empty text", () => {
    show(
      <HistoryFieldDiff
        field="title"
        oldValue={null}
        newValue="Head of Ops"
        values={{ currency: null, locale: "en", zone: "UTC" }}
      />,
    );
    expect(screen.queryByText("null")).toBeNull();
    expect(screen.getByText("Head of Ops")).toBeTruthy();
  });
});
