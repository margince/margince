// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordTabs } from "./recordtabs";

afterEach(cleanup);

type Body = "overview" | "people";
const LABELS: Record<Body, string> = { overview: "Overview", people: "People" };

function strip(trailing?: React.ReactNode) {
  return render(
    <LocaleProvider initial="en">
      <RecordTabs
        options={["overview", "people"]}
        value="overview"
        onChange={() => undefined}
        labels={LABELS}
        trailing={trailing}
      />
    </LocaleProvider>,
  );
}

// The row's far end is the page's control, not one more tab: it sits outside
// the strip a reader navigates, and a strip with nothing to put there draws
// no empty end.
describe("RecordTabs carries the details control at the row's end", () => {
  it("renders the trailing control outside the strip", () => {
    const { container } = strip(<button type="button">Details</button>);
    const trailing = container.querySelector(".recordtabs-trailing");
    expect(trailing).not.toBeNull();
    expect(
      trailing?.contains(screen.getByRole("button", { name: "Details" })),
    ).toBe(true);
    expect(
      container.querySelector(".recordtabs-strip")?.contains(trailing),
    ).toBe(false);
  });

  it("draws no end when there is nothing to put there", () => {
    const { container } = strip();
    expect(container.querySelector(".recordtabs-trailing")).toBeNull();
  });
});
