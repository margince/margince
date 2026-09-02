// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LocaleProvider } from "../i18n";
import { RowTags } from "./rowtags";

const tag = (id: string, name: string) => ({
  tag_id: id,
  name,
  color: null,
});

function draw(tags: ReturnType<typeof tag>[]) {
  render(
    <LocaleProvider initial="en">
      <RowTags tags={tags} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a row's tag strip", () => {
  it("draws the words a row carries", () => {
    draw([tag("t-1", "Key Account")]);
    expect(screen.getByText("Key Account")).toBeInTheDocument();
  });

  // The cap keeps one row one line high. Without it a record with twenty tags
  // pushes every row below it down the page.
  it("counts the words past the cap instead of drawing them", () => {
    draw([
      tag("t-1", "One"),
      tag("t-2", "Two"),
      tag("t-3", "Three"),
      tag("t-4", "Four"),
    ]);
    expect(screen.getByText("One")).toBeInTheDocument();
    expect(screen.getByText("Two")).toBeInTheDocument();
    expect(screen.queryByText("Three")).toBeNull();
    expect(screen.getByText(/\+2/)).toBeInTheDocument();
  });

  // A count with no words is a puzzle: the reader has to open the record to
  // learn what it hides.
  it("names the counted words rather than only counting them", () => {
    draw([
      tag("t-1", "One"),
      tag("t-2", "Two"),
      tag("t-3", "Three"),
      tag("t-4", "Four"),
    ]);
    expect(screen.getByText(/\+2/).getAttribute("title")).toBe("Three, Four");
  });

  // A per-row empty state would repeat one sentence fifty times down a page.
  it("draws nothing at all for a row with no tags", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <RowTags tags={[]} />
      </LocaleProvider>,
    );
    expect(container.querySelector(".rowtags")).toBeNull();
  });
});
