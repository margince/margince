/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import { DimensionChips } from "./reporting.dimensions";
import type { ReportingSelection } from "./reporting.model";

afterEach(cleanup);

function chips(selection: ReportingSelection) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <DimensionChips
          selection={selection}
          productLabel="Suite"
          initiativeLabel="DACH expansion"
          onChange={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const base = reportingStoryEvaluation.selection;

const SCOPE_LABELS: [ReportingSelection["scope"]["kind"], string][] = [
  ["workspace", "Company sales"],
  ["managed_teams", "Team sales"],
  ["team", "Team sales"],
  ["owner", "Seller sales"],
];

describe("the profile chip", () => {
  it.each(SCOPE_LABELS)("names a %s scope as %s", (kind, label) => {
    chips({ ...base, scope: { kind } });
    expect(screen.getByText(label)).toBeTruthy();
  });

  it("names an initiative or a product before the scope", () => {
    chips({ ...base, scope: { kind: "owner" }, initiative_id: "i-dach" });
    expect(screen.getByText("Sales initiative")).toBeTruthy();
    cleanup();
    chips({ ...base, scope: { kind: "owner" }, product_id: "p-suite" });
    expect(screen.getByText("Product sales")).toBeTruthy();
    expect(screen.queryByText("Seller sales")).toBeNull();
  });
});
