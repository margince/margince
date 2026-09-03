// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { MoneyPane } from "./glance";

afterEach(cleanup);

function drawMoney(loading: boolean) {
  return render(
    <LocaleProvider initial="en">
      <MoneyPane
        organizationId="o-1"
        loading={loading}
        readOnly={false}
        onAllDeals={() => undefined}
      />
    </LocaleProvider>,
  );
}

// The projects group reads its own state the way the deals group does. An
// absent list handed straight to the links section drew "No projects yet"
// with an Attach verb while the 360 was still on its way — an invitation to
// act on a section that had not answered.
describe("the money pane's projects group", () => {
  it("holds the loading state, not an empty plate, while the 360 is in flight", () => {
    drawMoney(true);
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
    // The group is still named, so a reader can tell WHICH reading is on its
    // way, one level under the pane's own title.
    expect(
      screen.getByRole("heading", { level: 3, name: "Projects" }),
    ).toBeTruthy();
  });

  it("says the section could not be read when the 360 failed", () => {
    drawMoney(false);
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
  });
});
