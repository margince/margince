/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { armHoverIntent } from "../design-system/hoverintent-testing";
import { ColumnGuide, ColumnGuideButton } from "./import.guide";
import { render } from "./import.testkit";

afterEach(cleanup);

describe("the column guide", () => {
  it("opens a column's guide on hover and on keyboard, and closes on Escape", async () => {
    const user = userEvent.setup();
    // The hover half is this case's subject, so the hook gets its real timer.
    armHoverIntent();
    render(
      <ColumnGuide
        file="companies.csv"
        column={{
          name: "domain",
          description: "The company's web domain.",
          example: "acme.com",
          required_on_new_row: false,
          custom: false,
        }}
      />,
    );
    await user.hover(screen.getByRole("button", { name: "domain" }));
    expect(await screen.findByText("The company's web domain.")).toBeVisible();
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByText("The company's web domain.")).toBeNull(),
    );
    screen.getByRole("button", { name: "domain" }).focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByText(/acme\.com/)).toBeVisible();
  });

  it("marks a column a new row needs", async () => {
    const user = userEvent.setup();
    render(
      <ColumnGuide
        file="deals.csv"
        column={{
          name: "pipeline",
          description: "d",
          example: "e",
          required_on_new_row: true,
          custom: false,
        }}
      />,
    );
    await user.click(screen.getByRole("button", { name: "pipeline" }));
    expect(await screen.findByText("Needed on a new row")).toBeVisible();
  });

  it("shows a custom field by its label and names its key and file", async () => {
    const user = userEvent.setup();
    render(
      <ColumnGuide
        file="companies.csv"
        column={{
          name: "cf_tier",
          label: "Tier",
          description: "A custom picklist.",
          example: "A",
          required_on_new_row: false,
          custom: true,
        }}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Tier" }));
    expect(
      await screen.findByText("cf_tier · companies.csv · custom field"),
    ).toBeVisible();
    expect(screen.queryByText("Needed on a new row")).toBeNull();
  });

  it("names the `?` beside a chosen field by the column it explains", async () => {
    const user = userEvent.setup();
    render(
      <ColumnGuideButton
        file="contacts.csv"
        column={{
          name: "company_ref",
          description: "Which company this row belongs to.",
          example: "CO-0042",
          required_on_new_row: false,
          custom: false,
        }}
      />,
    );
    await user.click(
      screen.getByRole("button", { name: "What goes in company_ref" }),
    );
    expect(
      await screen.findByText("Which company this row belongs to."),
    ).toBeVisible();
  });
});
