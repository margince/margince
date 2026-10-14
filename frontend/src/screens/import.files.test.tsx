/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatFixture,
  jsonResponse,
  render,
  run,
  stubRoutes,
  zipProfile,
} from "./import.testkit";
import { ImportWizardPage } from "./import.wizard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the files step", () => {
  // zipProfile: file_name "pipedrive-export.zip", suggested_source_name "pipedrive-export", parts companies.csv + contacts.csv
  it("detects each file's type, lets the reader change it, and refuses two files of one type", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
      "GET /imports/format": () => jsonResponse(formatFixture),
    });
    render(<ImportWizardPage />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["PK"], "pipedrive-export.zip"),
    );
    expect(await screen.findByDisplayValue("pipedrive-export")).toBeVisible();
    const companies = screen.getByRole("combobox", {
      name: "Record type for companies.csv",
    });
    expect(companies).toHaveTextContent("Companies");
    await user.click(
      screen.getByRole("combobox", { name: "Record type for contacts.csv" }),
    );
    await user.click(screen.getByRole("option", { name: "Companies" }));
    expect(screen.getByRole("button", { name: /Next/ })).toBeDisabled();
    expect(
      screen.getByText(/companies\.csv and contacts\.csv are both Companies/),
    ).toBeVisible();
  });

  it("sends a ZIP without a type and a CSV with the type its name gives", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
    });
    render(<ImportWizardPage />);
    const zone = await screen.findByLabelText("Upload your data");
    await user.upload(zone, new File(["PK"], "crm.zip"));
    await screen.findByDisplayValue("pipedrive-export");
    expect(sent.find((s) => s.path === "POST /imports/sources")?.body).toEqual({
      file: "crm.zip",
    });

    cleanup();
    globalThis.location.hash = "";
    const again = stubRoutes();
    render(<ImportWizardPage />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["display_name\n"], "Companies.csv"),
    );
    await waitFor(() =>
      expect(
        again.find((s) => s.path === "POST /imports/sources")?.body,
      ).toEqual({ object: "company", file: "Companies.csv" }),
    );
  });

  it("names the files it left out, and why", async () => {
    const user = userEvent.setup();
    stubRoutes({ "POST /imports/sources": () => jsonResponse(zipProfile) });
    render(<ImportWizardPage />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["PK"], "x.zip"),
    );
    expect(
      await screen.findByText("products.csv: the name matches no record type"),
    ).toBeVisible();
  });

  it("starts a ZIP's source name from the last ZIP run, so a re-download updates", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () => jsonResponse(zipProfile),
      "GET /imports": () =>
        jsonResponse({
          data: [
            { ...run, file_name: "leads.csv", source_name: "" },
            { ...run, file_name: "crm.zip", source_name: "crm" },
          ],
        }),
    });
    render(<ImportWizardPage />);
    const zone = await screen.findByLabelText("Upload your data");
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
    await user.upload(zone, new File(["PK"], "crm (1).zip"));
    expect(await screen.findByDisplayValue("crm")).toBeVisible();
  });

  it("refuses Next while no file has a type", async () => {
    const user = userEvent.setup();
    stubRoutes({ "POST /imports/sources": () => jsonResponse(zipProfile) });
    render(<ImportWizardPage />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["PK"], "x.zip"),
    );
    for (const file of ["companies.csv", "contacts.csv"]) {
      await user.click(
        await screen.findByRole("combobox", {
          name: `Record type for ${file}`,
        }),
      );
      await user.click(screen.getByRole("option", { name: "Skip this file" }));
    }
    expect(screen.getByRole("button", { name: /Next/ })).toBeDisabled();
    expect(
      screen.getByText("Choose a record type for at least one file."),
    ).toBeVisible();
  });
});
