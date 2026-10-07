// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { expectNameAndValueApart } from "./story-utils";

type Box = { left: number; top: number; width: number; height: number };

const NAME_LINE = { left: 0, top: 0, width: 40, height: 20 };

function rect({ left, top, width, height }: Box): DOMRect {
  return DOMRect.fromRect({ x: left, y: top, width, height });
}

const SPACE_2 = 8;

function fieldRow({
  display = "flex",
  columnGap = SPACE_2,
  value,
}: Readonly<{
  display?: string;
  columnGap?: number;
  value: Box;
}>): HTMLElement {
  const canvas = document.createElement("div");
  const row = document.createElement("li");
  row.className = "entry-field";
  row.style.display = display;
  row.style.setProperty("column-gap", `${columnGap}px`);
  row.style.setProperty("--space-2", `${SPACE_2}px`);
  const name = document.createElement("span");
  name.textContent = "Value";
  const diff = document.createElement("span");
  diff.textContent = "€25,000 → €41,500";
  row.append(name, diff);
  canvas.append(row);
  document.body.append(canvas);
  vi.spyOn(name, "getBoundingClientRect").mockReturnValue(rect(NAME_LINE));
  vi.spyOn(diff, "getBoundingClientRect").mockReturnValue(rect(value));
  return canvas;
}

afterEach(() => {
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe("expectNameAndValueApart", () => {
  it("throws a readable error when no field row holds the name", async () => {
    const canvas = document.createElement("div");
    const name = document.createElement("span");
    name.textContent = "Value";
    canvas.append(name);
    document.body.append(canvas);
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      'the field named "Value" is not drawn as an .entry-field row holding a name and a value',
    );
  });

  it("throws a readable error when the row holds the name alone", async () => {
    const canvas = fieldRow({ value: { ...NAME_LINE, left: 48 } });
    canvas.querySelector(".entry-field > :last-child")?.remove();
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      "is not drawn as an .entry-field row holding a name and a value",
    );
  });

  it("fails a row the cascade left as a block", async () => {
    const canvas = fieldRow({
      display: "block",
      value: { ...NAME_LINE, left: 48 },
    });
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      "expected 'block' to be 'flex'",
    );
  });

  it("fails a row whose column gap is under --space-2", async () => {
    const canvas = fieldRow({
      columnGap: SPACE_2 / 2,
      value: { ...NAME_LINE, left: 48 },
    });
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      "expected 4 to be greater than or equal to 8",
    );
  });

  it("passes a value beside its name with a full gap between", async () => {
    const canvas = fieldRow({ value: { ...NAME_LINE, left: 48 } });
    await expect(expectNameAndValueApart(canvas)).resolves.toBeUndefined();
  });

  it("fails a value beside its name with less than a full gap", async () => {
    const canvas = fieldRow({ value: { ...NAME_LINE, left: 44 } });
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      "expected 4 to be greater than or equal to 7.5",
    );
  });

  it("passes a value wrapped directly under its name", async () => {
    const canvas = fieldRow({ value: { ...NAME_LINE, top: 20 } });
    await expect(expectNameAndValueApart(canvas)).resolves.toBeUndefined();
  });

  it("fails a value wrapped under its name but set off to one side", async () => {
    const canvas = fieldRow({ value: { ...NAME_LINE, left: 6, top: 20 } });
    await expect(expectNameAndValueApart(canvas)).rejects.toThrow(
      "expected 6 to be less than or equal to 1",
    );
  });
});
