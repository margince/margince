/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { Heading } from "./heading";
import { Modal } from "./modal";
import { MultiSelect, Select, type SelectOption } from "./select";
import { FILTER_FROM } from "./selectpopup";

afterEach(cleanup);

// A list long enough to scroll opens with a box that narrows it. A reader finds
// a colleague among hundreds by any part of the name or email.
describe("a long list", () => {
  const COLLEAGUES: readonly SelectOption[] = Array.from(
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
          options={COLLEAGUES}
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
    expect(screen.getAllByRole("option")).toHaveLength(COLLEAGUES.length);
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
    expect(screen.getAllByRole("option")).toHaveLength(COLLEAGUES.length);
  });

  // The trigger is the page's only stop, so one Tab leaves the page from it.
  it.each([false, true])(
    "hands Tab to the browser from the trigger when no stop is beside it (shift: %s)",
    async (shiftKey) => {
      const user = userEvent.setup();
      const trigger = renderLong();
      await user.click(trigger);
      const box = screen.getByRole("combobox", { name: "Filter the list" });

      expect(fireEvent.keyDown(box, { key: "Tab", shiftKey })).toBe(true);
      expect(trigger).toHaveFocus();
    },
  );

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "keeps IME confirmation in the filter: %o",
    async (composition) => {
      const changes: string[] = [];
      const user = userEvent.setup();
      await user.click(renderLong((value) => changes.push(value)));
      const input = screen.getByRole("combobox", { name: "Filter the list" });
      fireEvent.keyDown(input, { key: "Enter", ...composition });
      expect(changes).toEqual([]);
      expect(input).toHaveFocus();
      expect(
        screen.getByRole("listbox", { name: "Owner" }),
      ).toBeInTheDocument();
    },
  );

  it("keeps an applied filter editable when the roster shrinks", async () => {
    const user = userEvent.setup();
    const view = (options: readonly SelectOption[]) => (
      <LocaleProvider initial="en">
        <Select
          aria-label="Owner"
          options={options}
          value=""
          onChange={() => {}}
        />
      </LocaleProvider>
    );
    const { rerender } = render(view(COLLEAGUES));
    await user.click(screen.getByRole("combobox", { name: "Owner" }));
    await user.keyboard("mira");
    rerender(view(COLLEAGUES.slice(-2)));
    const input = screen.getByRole("combobox", { name: "Filter the list" });
    expect(input).toHaveFocus();
    await user.clear(input);
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(input).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("combobox", { name: "Owner" })).toHaveFocus();
  });

  it("tabs out of the filtered select inside a modal", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <Modal open intent="form" onClose={() => {}} labelledBy="edit-title">
          <Heading size="large" id="edit-title">
            Edit
          </Heading>
          <Select
            aria-label="Owner"
            options={COLLEAGUES}
            value=""
            onChange={() => {}}
          />
          <button type="button">After</button>
        </Modal>
      </LocaleProvider>,
    );
    await user.click(screen.getByRole("combobox", { name: "Owner" }));
    await user.tab();
    expect(screen.getByRole("button", { name: "After" })).toHaveFocus();
  });

  it("names the sibling multi-value list from its field", async () => {
    const user = userEvent.setup();
    render(
      <MultiSelect
        aria-label="Tags"
        options={COLLEAGUES}
        values={[]}
        onChange={() => {}}
      />,
    );
    await user.click(screen.getByRole("combobox", { name: "Tags" }));
    expect(screen.getByRole("listbox", { name: "Tags" })).toBeInTheDocument();
  });

  it("keeps a short list free of a box", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <Select
          aria-label="Owner"
          options={COLLEAGUES.slice(0, FILTER_FROM - 1)}
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
