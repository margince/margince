// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { JsonField, lineOfPath, parseProblem } from "./jsonfield";

afterEach(cleanup);

const TEXT = `{
  "provider": {
    "sort": { "by": "fastest" },
    "only": ["a", "b"],
    "zdr": true
  }
}`;

describe("lineOfPath", () => {
  it("walks each key in order to the line that holds the last", () => {
    expect(lineOfPath(TEXT, "provider.sort.by")).toBe(3);
    expect(lineOfPath(TEXT, "provider.zdr")).toBe(5);
    expect(lineOfPath(TEXT, "provider.only[1]")).toBe(4);
  });

  it("points at the deepest key it found when a later one is missing", () => {
    expect(lineOfPath(TEXT, "provider.order")).toBe(2);
    expect(lineOfPath(TEXT, "reasoning.effort")).toBeUndefined();
  });
});

describe("parseProblem", () => {
  it("is null for valid or empty text", () => {
    expect(parseProblem(TEXT)).toBeNull();
    expect(parseProblem("  ")).toBeNull();
  });

  it("names the line the parser stopped at", () => {
    const problem = parseProblem(`{\n  "a": 1,\n}`);
    expect(problem).toEqual({ line: 3 });
  });
});

function Harness({ problemLines = [] }: Readonly<{ problemLines?: number[] }>) {
  const [value, setValue] = useState(`{\n}`);
  return (
    <JsonField
      aria-label="json"
      value={value}
      onChange={setValue}
      problemLines={problemLines}
      invalid={problemLines.length > 0}
    />
  );
}

describe("JsonField", () => {
  it("numbers every line and marks the problem lines", () => {
    const { container } = render(<Harness problemLines={[2]} />);
    const numbers = [
      ...container.querySelectorAll(".json-field-gutter > span"),
    ];
    expect(numbers.map((n) => n.textContent)).toEqual(["1", "2"]);
    expect(numbers[1].className).toBe("json-field-mark");
    expect(screen.getByRole("textbox", { name: "json" })).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("indents on Tab instead of leaving the field", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const box = screen.getByRole("textbox", { name: "json" });
    await user.click(box);
    await user.keyboard("{Tab}");
    expect(box).toHaveValue(`{\n}  `);
    expect(box).toHaveFocus();
  });
});
