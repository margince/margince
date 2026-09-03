// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
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
  // learn what it hides. On hover AND on focus, because an answer reachable
  // only by pointer is one half the readers cannot get to.
  it("names the counted words rather than only counting them", async () => {
    draw([tag("t-1", "One"), tag("t-2", "Two"), tag("t-3", "Three")]);
    const rest = screen.getByText(/\+1/);
    rest.focus();
    expect(await screen.findByText("Three")).toBeInTheDocument();
  });

  // AT the wire cap the array stopped early, so the strip does not know how
  // many are left. It says "more" rather than a number it cannot stand behind
  // — a record with forty tags drawn as "+3" tells the reader something false.
  it("counts no number when the wire's answer stopped at the cap", () => {
    draw([
      tag("t-1", "One"),
      tag("t-2", "Two"),
      tag("t-3", "Three"),
      tag("t-4", "Four"),
      tag("t-5", "Five"),
    ]);
    expect(screen.queryByText(/\+3/)).toBeNull();
    expect(screen.getByText(en["tags.moreUncounted"])).toBeInTheDocument();
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
