/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SetPicker, type SetPickerOption } from "./setpicker";

afterEach(cleanup);

const TEAMS: SetPickerOption[] = [
  { value: "t1", label: "DACH Sales", detail: "4 members" },
  { value: "t2", label: "Nordics Delivery" },
  { value: "t3", label: "Benelux Sales" },
];

function Picker({
  options = TEAMS,
  initial = [],
  onCreate,
}: Readonly<{
  options?: SetPickerOption[];
  initial?: string[];
  onCreate?: (name: string) => void;
}>) {
  const [values, setValues] = useState(initial);
  return (
    <SetPicker
      legend="Teams"
      options={options}
      values={values}
      onChange={setValues}
      none="Not on a team yet."
      addLabel="Add teams"
      searchLabel="Search teams"
      noMatch="No team matches."
      doneLabel="Done"
      empty="Type a name to create the first team."
      removeLabel={(item) => `Remove from ${item.label}`}
      create={
        onCreate && { label: (name) => `Create team “${name}”`, onCreate }
      }
    />
  );
}

const tokens = () =>
  within(screen.getByRole("group", { name: "Teams" }))
    .queryAllByRole("listitem")
    .map((item) => item.textContent);

describe("SetPicker", () => {
  it("shows the chosen values as tokens, and says so when there are none", () => {
    render(<Picker initial={["t3", "t1"]} />);
    expect(tokens()).toEqual(["Benelux Sales", "DACH Sales"]);
    cleanup();
    render(<Picker />);
    expect(screen.getByText("Not on a team yet.")).toBeTruthy();
  });

  it("narrows the list to the typed words and ticks a value into the set", async () => {
    const user = userEvent.setup();
    render(<Picker />);
    await user.click(screen.getByRole("button", { name: /Add teams/ }));
    await user.type(screen.getByRole("searchbox"), "sales");
    expect(screen.getAllByRole("checkbox")).toHaveLength(2);
    await user.click(screen.getByRole("checkbox", { name: /Benelux Sales/ }));
    expect(tokens()).toEqual(["Benelux Sales"]);
    await user.clear(screen.getByRole("searchbox"));
    await user.type(screen.getByRole("searchbox"), "zzz");
    expect(screen.getByText("No team matches.")).toBeTruthy();
  });

  it("removes a value from its token", async () => {
    const user = userEvent.setup();
    render(<Picker initial={["t1", "t2"]} />);
    await user.click(
      screen.getByRole("button", { name: "Remove from DACH Sales" }),
    );
    expect(tokens()).toEqual(["Nordics Delivery"]);
  });

  it("offers a typed name as new only when no value carries it already", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    render(<Picker onCreate={onCreate} />);
    await user.click(screen.getByRole("button", { name: /Add teams/ }));
    await user.type(screen.getByRole("searchbox"), "dach SALES");
    expect(screen.queryByRole("button", { name: /Create team/ })).toBeNull();
    await user.clear(screen.getByRole("searchbox"));
    await user.type(screen.getByRole("searchbox"), " Iberia ");
    await user.click(
      screen.getByRole("button", { name: "Create team “Iberia”" }),
    );
    expect(onCreate).toHaveBeenCalledWith("Iberia");
  });

  it("says how to start when there is nothing to choose", async () => {
    const user = userEvent.setup();
    render(<Picker options={[]} onCreate={() => undefined} />);
    await user.click(screen.getByRole("button", { name: /Add teams/ }));
    expect(
      screen.getByText("Type a name to create the first team."),
    ).toBeTruthy();
  });

  it("draws a read-only set with no way to change it", () => {
    render(
      <SetPicker
        legend="Teams"
        options={TEAMS}
        values={["t1"]}
        onChange={() => undefined}
        none="Not on a team yet."
        addLabel="Add teams"
        searchLabel="Search teams"
        noMatch="No team matches."
        doneLabel="Done"
        removeLabel={(item) => `Remove from ${item.label}`}
        disabled
      />,
    );
    expect(tokens()).toEqual(["DACH Sales"]);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });
});
