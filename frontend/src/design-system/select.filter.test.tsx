/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { Select, type SelectOption } from "./select";
import { FILTER_FROM } from "./selectpopup";

afterEach(cleanup);

// A list long enough to scroll opens with a box that narrows it, so a reader
// finds a colleague among hundreds by any part of the name or email.
describe("a long list", () => {
  const PEOPLE: readonly SelectOption[] = Array.from(
    { length: 14 },
    (_, i) => ({
      value: `u-${i}`,
      label: `Member ${String(i).padStart(2, "0")}`,
      keywords: [`member${i}@example.test`],
    }),
  ).concat([
    {
      value: "u-mira",
      label: "Mira Voss",
      keywords: ["mira.voss@example.test"],
    },
  ]);

  function renderLong(onChange: (value: string) => void = () => {}) {
    render(
      <LocaleProvider initial="en">
        <Select
          aria-label="Owner"
          options={PEOPLE}
          value=""
          onChange={onChange}
        />
      </LocaleProvider>,
    );
    return screen.getByRole("combobox", { name: "Owner" });
  }

  it("opens with the filter box focused", async () => {
    const user = userEvent.setup();
    await user.click(renderLong());

    expect(
      screen.getByRole("combobox", { name: "Filter the list" }),
    ).toHaveFocus();
    expect(screen.getAllByRole("option")).toHaveLength(PEOPLE.length);
  });

  it("narrows by a part of the name, ignoring case and accents", async () => {
    const user = userEvent.setup();
    await user.click(renderLong());
    await user.keyboard("vöss");

    expect(screen.getAllByRole("option").map((row) => row.textContent)).toEqual(
      ["Mira Voss"],
    );
  });

  it("narrows by the email a keyword carries", async () => {
    const user = userEvent.setup();
    await user.click(renderLong());
    await user.keyboard("mira.voss@");

    expect(
      screen.getByRole("option", { name: "Mira Voss" }),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(1);
  });

  it("picks the first row left with Enter, and closes on the trigger", async () => {
    const changes: string[] = [];
    const user = userEvent.setup();
    const trigger = renderLong((value) => changes.push(value));
    await user.click(trigger);
    await user.keyboard("mira{Enter}");

    expect(changes).toEqual(["u-mira"]);
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it("says so when nothing matches", async () => {
    const user = userEvent.setup();
    await user.click(renderLong());
    await user.keyboard("zzz");

    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByRole("status")).toHaveTextContent("No match");
  });

  it("starts the next open on the whole list", async () => {
    const user = userEvent.setup();
    const trigger = renderLong();
    await user.click(trigger);
    await user.keyboard("mira{Escape}");
    await user.click(trigger);

    expect(
      screen.getByRole("combobox", { name: "Filter the list" }),
    ).toHaveValue("");
    expect(screen.getAllByRole("option")).toHaveLength(PEOPLE.length);
  });

  it("keeps a short list free of a box", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <Select
          aria-label="Owner"
          options={PEOPLE.slice(0, FILTER_FROM - 1)}
          value=""
          onChange={() => {}}
        />
      </LocaleProvider>,
    );
    await user.click(screen.getByRole("combobox", { name: "Owner" }));

    expect(
      screen.queryByRole("combobox", { name: "Filter the list" }),
    ).toBeNull();
  });
});
