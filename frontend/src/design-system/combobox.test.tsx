/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { ComboBox } from "./combobox";

const MODELS = [
  { value: "gemini-3.5-flash" },
  { value: "gemini-3.1-flash-lite" },
  { value: "gemini-3.1-pro-preview" },
];

function Harness({
  suggestions = MODELS,
  initial = "",
}: Readonly<{
  suggestions?: readonly { value: string; hint?: string }[];
  initial?: string;
}>) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <ComboBox
        aria-label="Model"
        value={value}
        onChange={setValue}
        suggestions={suggestions}
      />
      <p data-testid="committed">{value}</p>
    </>
  );
}

describe("ComboBox", () => {
  afterEach(cleanup);

  it("keeps a value the suggestions do not offer", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(
      screen.getByRole("combobox", { name: "Model" }),
      "my-own-model",
    );

    expect(screen.getByTestId("committed")).toHaveTextContent("my-own-model");
  });

  it("commits the suggestion a reader picks", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const box = screen.getByRole("combobox", { name: "Model" });
    await user.click(box);
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "gemini-3.1-flash-lite",
      }),
    );

    expect(screen.getByTestId("committed")).toHaveTextContent(
      "gemini-3.1-flash-lite",
    );
    expect(box).toHaveValue("gemini-3.1-flash-lite");
  });

  it("narrows the list to what has been typed, and still keeps the typing", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(screen.getByRole("combobox", { name: "Model" }), "flash");

    const options = within(screen.getByRole("listbox")).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual([
      "gemini-3.5-flash",
      "gemini-3.1-flash-lite",
    ]);
    expect(screen.getByTestId("committed")).toHaveTextContent("flash");
  });

  // The half that separates this from a Select: what a reader typed is theirs,
  // and closing the list is not a reason to lose it.
  it("keeps what was typed when Escape closes the list", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const box = screen.getByRole("combobox", { name: "Model" });
    await user.type(box, "gemini-4-experimental");
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(box).toHaveValue("gemini-4-experimental");
    expect(screen.getByTestId("committed")).toHaveTextContent(
      "gemini-4-experimental",
    );
  });

  // An installation whose price sheet knows nothing about the chosen provider.
  // The field still works; it just has nothing to offer.
  it("is a plain text box when there is nothing to suggest", async () => {
    const user = userEvent.setup();
    render(<Harness suggestions={[]} />);

    const box = screen.getByRole("combobox", { name: "Model" });
    await user.click(box);
    await user.type(box, "anything");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.getByTestId("committed")).toHaveTextContent("anything");
  });

  it("walks the list from the keyboard and commits on Enter", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const box = screen.getByRole("combobox", { name: "Model" });
    await user.click(box);
    await user.keyboard("{ArrowDown}{ArrowDown}");

    const active = box.getAttribute("aria-activedescendant");
    expect(active).toBeTruthy();
    expect(document.getElementById(active ?? "")).toHaveTextContent(
      "gemini-3.1-flash-lite",
    );

    await user.keyboard("{Enter}");
    expect(screen.getByTestId("committed")).toHaveTextContent(
      "gemini-3.1-flash-lite",
    );
  });

  // A bound model the sheet no longer prices: it is the value, and the priced
  // ones are still offered beside it.
  it("shows a value that is not among the suggestions", async () => {
    const user = userEvent.setup();
    render(<Harness initial="a-model-nobody-priced" />);

    const box = screen.getByRole("combobox", { name: "Model" });
    expect(box).toHaveValue("a-model-nobody-priced");

    await user.clear(box);
    expect(
      within(screen.getByRole("listbox")).getAllByRole("option"),
    ).toHaveLength(MODELS.length);
  });

  it("refuses every interaction when disabled", async () => {
    const user = userEvent.setup();
    render(
      <ComboBox
        aria-label="Model"
        value="gemini-3.5-flash"
        onChange={() => {
          throw new Error("a disabled combo box must not report a change");
        }}
        suggestions={MODELS}
        disabled
      />,
    );

    const box = screen.getByRole("combobox", { name: "Model" });
    expect(box).toBeDisabled();
    await user.click(box);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
});
