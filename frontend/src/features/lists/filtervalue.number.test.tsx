// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { LeafValue } from "../../lib/segmentpredicate";
import { StoryProviders } from "../../testing/story-utils";
import { ValueControl } from "./filtervalue";

afterEach(cleanup);

// A number clause in German: "12,5" is twelve and a half. "1.500" stays as
// typed with the reason under it, so the clause holds its preview back.
it("reads a German number operand, and refuses 1.500", async () => {
  const seen: LeafValue[] = [];
  render(
    <StoryProviders locale="de">
      <ValueControl
        type="number"
        references={undefined}
        options={undefined}
        op="gte"
        value=""
        label="Punkte"
        onChange={(next) => seen.push(next)}
      />
    </StoryProviders>,
  );
  const box = await screen.findByLabelText("Punkte");
  fireEvent.change(box, { target: { value: "12,5" } });
  expect(seen.at(-1)).toBe(12.5);
  fireEvent.change(box, { target: { value: "1.500" } });
  expect(seen.at(-1)).toBe("1.500");
  expect(
    await screen.findByText("Gib die Zahl als 1500 oder 1,5 ein."),
  ).toBeTruthy();
});

function NumberList({
  start,
  seen,
}: Readonly<{ start: LeafValue; seen: LeafValue[] }>) {
  const [value, setValue] = useState<LeafValue>(start);
  return (
    <ValueControl
      type="number"
      references={undefined}
      options={undefined}
      op="in"
      value={value}
      label="Points"
      onChange={(next) => {
        seen.push(next);
        setValue(next);
      }}
    />
  );
}

describe("a number clause's list", () => {
  it("writes the numbers typed, and refuses a token that is not one", async () => {
    const seen: LeafValue[] = [];
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <NumberList start={[]} seen={seen} />
      </StoryProviders>,
    );
    const box = screen.getByRole("textbox", { name: "Points" });
    await user.type(box, "12{Enter}");
    expect(seen.at(-1)).toEqual([12]);

    await user.type(box, "abc{Enter}");
    expect(seen.at(-1)).toEqual([12]);
    expect(box).toHaveValue("abc");
    expect(
      await screen.findByText("Enter a number, such as 1234.5."),
    ).toBeTruthy();
  });

  it("shows the numbers it holds the reader's way", () => {
    render(
      <StoryProviders locale="de">
        <NumberList start={[12.5]} seen={[]} />
      </StoryProviders>,
    );
    expect(screen.getByText("12,5")).toBeTruthy();
  });
});
