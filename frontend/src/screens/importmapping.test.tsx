/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatFixture,
  jsonResponse,
  render,
  stubRoutes,
  zipProfile,
} from "./import.testkit";
import { ImportWizardPage } from "./import.wizard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

async function toTheMap(user: UserEvent) {
  render(<ImportWizardPage />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["PK"], "x.zip"),
  );
  await user.click(
    await screen.findByRole("button", { name: /Next: map 2 files/ }),
  );
}

describe("the map step", () => {
  it("maps template headers by themselves and opens a field's guide from the map", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
      "GET /imports/format": () => jsonResponse(formatFixture),
    });
    await toTheMap(user);
    await user.click(screen.getByRole("button", { name: /Contacts/ }));
    expect(
      screen.getByRole("combobox", { name: "Field for company_ref" }),
    ).toHaveTextContent("company_ref");
    await user.click(
      screen.getByRole("button", { name: "What goes in company_ref" }),
    );
    expect(
      await screen.findByText(/Which company this row belongs to/),
    ).toBeVisible();
  });

  it("stages a custom field by its key, never its label", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
    });
    await toTheMap(user);
    await user.click(screen.getByRole("combobox", { name: "Field for Level" }));
    await user.click(screen.getByRole("option", { name: "Tier" }));
    await user.click(screen.getByRole("button", { name: "Run test" }));
    await waitFor(() =>
      expect(sent.some((s) => s.path === "POST /imports")).toBe(true),
    );
    expect(sent.find((s) => s.path === "POST /imports")?.body).toEqual({
      connector: "csv",
      source_ref: "ws/import/zip",
      parts: [
        {
          file: "companies.csv",
          object: "company",
          mapping: {
            ref: "ref",
            display_name: "display_name",
            Level: "cf_tier",
          },
        },
        {
          file: "contacts.csv",
          object: "contact",
          mapping: { ref: "ref", email: "email", company_ref: "company_ref" },
        },
      ],
      source_name: "pipedrive-export",
      on_broken_link: "skip_row",
      file_name: "pipedrive-export.zip",
    });
  });

  it("orders the tabs by the format and counts the columns each leaves out", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
      "GET /imports/format": () =>
        jsonResponse({
          ...formatFixture,
          order: ["contact", "company", "lead"],
        }),
    });
    await toTheMap(user);
    const strip = screen.getByRole("group", { name: "Files to map" });
    const tabs = within(strip).getAllByRole("button");
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      "Contacts",
      "Companies, 1",
    ]);
  });

  it("starts a file's mapping again from its new type's proposal", async () => {
    const user = userEvent.setup();
    stubRoutes({ "POST /imports/sources": () => jsonResponse(zipProfile) });
    render(<ImportWizardPage />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["PK"], "x.zip"),
    );
    await user.click(
      await screen.findByRole("combobox", {
        name: "Record type for contacts.csv",
      }),
    );
    await user.click(screen.getByRole("option", { name: "Prospects" }));
    await user.click(screen.getByRole("button", { name: /Next: map 2 files/ }));
    await user.click(screen.getByRole("button", { name: /Prospects/ }));
    expect(
      screen.getByRole("combobox", { name: "Field for company_ref" }),
    ).toHaveTextContent("Do not import");
    expect(
      screen.getByRole("combobox", { name: "Field for email" }),
    ).toHaveTextContent("email");
  });
});
