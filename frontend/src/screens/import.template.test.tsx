/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ImportCard } from "./import";
import {
  formatFixture,
  jsonResponse,
  render,
  stubRoutes,
} from "./import.testkit";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the template preview", () => {
  // formatFixture's custom column is { name: "cf_tier", label: "Tier", custom: true, … }
  it("shows every file with its columns and custom fields from the format", async () => {
    stubRoutes({ "GET /imports/format": () => jsonResponse(formatFixture) });
    render(<ImportCard subpage="template" />);
    expect(await screen.findByText("companies.csv")).toBeVisible();
    expect(screen.getByRole("button", { name: "Tier" })).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Download template" }),
    ).toHaveAttribute("href", "/v1/imports/template");
  });

  it("lists the files in the format's order, not the screen's", async () => {
    stubRoutes({
      "GET /imports/format": () =>
        jsonResponse({
          ...formatFixture,
          files: [...formatFixture.files].reverse(),
        }),
    });
    render(<ImportCard subpage="template" />);
    await screen.findByText("companies.csv");
    const files = screen
      .getAllByRole("code")
      .map((cell) => cell.textContent ?? "");
    expect(files).toEqual(["leads.csv", "contacts.csv", "companies.csv"]);
  });

  it("asks the format in the reader's language and shows its key rules", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(<ImportCard subpage="template" />);
    await screen.findByText("companies.csv");
    expect(sent.map((s) => s.path)).toContain("GET /imports/format");
    await user.click(screen.getByText("How rows are matched"));
    expect(screen.getByText(formatFixture.key_rules[0] ?? "")).toBeVisible();
  });
});
