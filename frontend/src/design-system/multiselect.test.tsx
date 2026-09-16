/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { SelectOption } from "./select";
import { MultiSelect } from "./select";
import { toggleOptions } from "./select-testing";

// The specs for the multi-value dropdown. Select's own suite already holds the
// shared anatomy — the combobox trigger, the portalled listbox, the keyboard
// walk — so what lives here is only what MANY values change: a pick toggles
// membership instead of committing, the list survives it, and the closed face
// reads the whole set.

afterEach(cleanup);

const STAGES: readonly SelectOption[] = [
  { value: "qualify", label: "Qualify" },
  { value: "proposal", label: "Proposal" },
  { value: "won", label: "Won" },
];

function renderMulti(start: readonly string[] = [], placeholder?: string) {
  const committed: string[][] = [];
  function Harness() {
    const [values, setValues] = useState<string[]>([...start]);
    return (
      <MultiSelect
        aria-label="Stages"
        options={STAGES}
        values={values}
        onChange={(next) => {
          committed.push(next);
          setValues(next);
        }}
        placeholder={placeholder}
      />
    );
  }
  render(<Harness />);
  return {
    committed,
    trigger: screen.getByRole("combobox", { name: "Stages" }),
  };
}

describe("toggling", () => {
  it("adds on first pick, removes on the second, and the list stays open throughout", async () => {
    const user = userEvent.setup();
    const { committed, trigger } = renderMulti();

    await user.click(trigger);
    const listbox = screen.getByRole("listbox");
    expect(listbox.getAttribute("aria-multiselectable")).toBe("true");

    await user.click(within(listbox).getByRole("option", { name: "Won" }));
    expect(committed).toEqual([["won"]]);
    // The popup survives the pick — the whole reason this control exists.
    expect(screen.getByRole("listbox")).toBeTruthy();
    expect(
      screen.getByRole("option", { name: "Won" }).getAttribute("aria-selected"),
    ).toBe("true");

    await user.click(screen.getByRole("option", { name: "Won" }));
    expect(committed).toEqual([["won"], []]);
    expect(
      screen.getByRole("option", { name: "Won" }).getAttribute("aria-selected"),
    ).toBe("false");
  });

  it("keeps the keyboard on the trigger across a mouse toggle, so Escape still closes", async () => {
    const user = userEvent.setup();
    const { trigger } = renderMulti();

    await user.click(trigger);
    await user.click(screen.getByRole("option", { name: "Qualify" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("toggles from the keyboard too, and Enter does not close the list", async () => {
    const user = userEvent.setup();
    const { committed, trigger } = renderMulti();

    trigger.focus();
    await user.keyboard("{ArrowDown}{Enter}");
    expect(committed).toEqual([["qualify"]]);
    expect(screen.getByRole("listbox")).toBeTruthy();

    await user.keyboard("{ArrowDown}{Enter}");
    expect(committed).toEqual([["qualify"], ["qualify", "proposal"]]);
  });
});

describe("the closed face", () => {
  it("reads the chosen labels in OPTION order, however they were clicked", async () => {
    const user = userEvent.setup();
    const { trigger } = renderMulti();

    await toggleOptions(user, trigger, ["Won", "Qualify"]);
    expect(trigger.textContent).toContain("Qualify, Won");
  });

  it("rests on the placeholder while nothing is chosen", () => {
    const { trigger } = renderMulti([], "Not set");
    expect(trigger.textContent).toContain("Not set");
  });

  it("shows a prefilled set without any interaction", () => {
    const { trigger } = renderMulti(["proposal", "won"]);
    expect(trigger.textContent).toContain("Proposal, Won");
  });
});

describe("toggleOptions", () => {
  it("drives several toggles in one open-close round and reports the set it left", async () => {
    const user = userEvent.setup();
    const { committed, trigger } = renderMulti(["qualify"]);

    await toggleOptions(user, trigger, ["Won", "Qualify"]);
    expect(committed.at(-1)).toEqual(["won"]);
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("fails loudly on a label the list does not offer", async () => {
    const user = userEvent.setup();
    const { trigger } = renderMulti();

    await expect(toggleOptions(user, trigger, ["Renewal"])).rejects.toThrow();
  });
});
