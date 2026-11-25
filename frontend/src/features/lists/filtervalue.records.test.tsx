// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import type { LeafValue } from "../../lib/segmentpredicate";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SearchedRecordValue } from "./filtervalue.records";

afterEach(cleanup);

const ID = "01a1e510-c819-70a4-80a4-700299ba3784";

describe("a company clause restored from a saved view", () => {
  it("shows the company's name and names its remove button, never the id", async () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      ...recordNamesRoutes({ company: { [ID]: "Demo GmbH" } }),
    });
    render(
      <StoryProviders>
        <SearchedRecordValue
          many
          value={[ID]}
          onChange={vi.fn()}
          placeholder={en["filters.pickCompany"]}
        />
      </StoryProviders>,
    );
    expect(await screen.findByText("Demo GmbH")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove Demo GmbH" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(ID)).not.toBeInTheDocument();
  });
});

function OneCompany() {
  const [value, setValue] = useState<LeafValue>("");
  return (
    <>
      <SearchedRecordValue
        many={false}
        value={value}
        onChange={setValue}
        placeholder={en["filters.pickCompany"]}
      />
      <output>{JSON.stringify(value)}</output>
      <button type="button" onClick={() => setValue("")}>
        Clear from outside
      </button>
    </>
  );
}

describe("searching for a company", () => {
  it("offers the answers as a suggestion list the keyboard walks", async () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      "GET /companies": () =>
        jsonResponse({
          data: [
            { id: ID, display_name: "Demo GmbH" },
            { id: "c-2", display_name: "Demo Labs" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ...recordNamesRoutes({ company: { [ID]: "Demo GmbH" } }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <OneCompany />
      </StoryProviders>,
    );

    const box = screen.getByRole("combobox", {
      name: en["filters.searchRecords"],
    });
    expect(box).toHaveAttribute("placeholder", en["filters.pickCompany"]);
    await user.type(box, "Demo");
    const list = await screen.findByRole("listbox");
    expect(
      within(list)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Demo GmbH", "Demo Labs"]);

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(box).toHaveValue("Demo");
    expect(screen.getByRole("status", { hidden: true })).toHaveTextContent(
      '""',
    );

    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByRole("status", { hidden: true })).toHaveTextContent(
      JSON.stringify(ID),
    );
    // The name stands where the box was, and focus moves to the way back.
    expect(screen.getByText("Demo GmbH")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["filters.changeRecord"] }),
    ).toHaveFocus();
  });

  it("moves focus only after the reader's own pick, not on a later outside change", async () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      "GET /companies": () =>
        jsonResponse({
          data: [{ id: ID, display_name: "Demo GmbH" }],
          page: { next_cursor: null, has_more: false },
        }),
      ...recordNamesRoutes({ company: { [ID]: "Demo GmbH" } }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <OneCompany />
      </StoryProviders>,
    );
    await user.type(
      screen.getByRole("combobox", { name: en["filters.searchRecords"] }),
      "Demo",
    );
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(
      screen.getByRole("button", { name: en["filters.changeRecord"] }),
    ).toHaveFocus();

    const outside = screen.getByRole("button", { name: "Clear from outside" });
    await user.click(outside);
    expect(
      screen.getByRole("combobox", { name: en["filters.searchRecords"] }),
    ).not.toHaveFocus();
    expect(outside).toHaveFocus();
  });
});
