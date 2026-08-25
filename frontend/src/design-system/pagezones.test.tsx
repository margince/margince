/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PageZones } from "./pagezones";

// The spec for the layout's READING order, which is the one promise the grid
// cannot keep on its own and the one nobody sees by looking at the page: a
// screen reader and the tab key walk the DOM, so the work column comes first
// there at every width, whatever the columns do visually (WCAG 2.2 §1.3.2).
// The shape this forbids is a rail that arrives ahead of the record, placed
// back under it with an `order` — which reads correctly and tabs backwards.
//
// jsdom applies no stylesheet, so this asserts the structure rather than the
// picture: the picture is `pagezones.stories.tsx`, including the folded width.

afterEach(cleanup);

const WORK = "What is happening";
const PROFILE = "Profile";
const CONTEXT = "Context";

function zones(): HTMLElement[] {
  const grid = screen.getByText(WORK).parentElement?.parentElement;
  if (!grid) {
    throw new Error("the work column is not inside a grid container");
  }
  return [...grid.children].filter(
    (zone): zone is HTMLElement => zone instanceof HTMLElement,
  );
}

function bothRails() {
  return (
    <PageZones
      shape="both"
      main={<p>{WORK}</p>}
      rail={<p>who they are</p>}
      railLabel={PROFILE}
      aside={<p>what it is worth</p>}
      asideLabel={CONTEXT}
    />
  );
}

describe("PageZones reading order", () => {
  it("puts the work column first with both rails", () => {
    render(bothRails());
    expect(zones().map((zone) => zone.getAttribute("aria-label"))).toEqual([
      null,
      PROFILE,
      CONTEXT,
    ]);
  });

  it("puts the work column first with only the left rail", () => {
    render(
      <PageZones
        shape="rail"
        main={<p>{WORK}</p>}
        rail={<p>who they are</p>}
        railLabel={PROFILE}
      />,
    );
    expect(zones().map((zone) => zone.getAttribute("aria-label"))).toEqual([
      null,
      PROFILE,
    ]);
  });

  it("hands each column the class the stylesheet places it by", () => {
    render(bothRails());
    // Without a class per column the only way to draw the left rail to the
    // left of a work column that precedes it is `order`, and an `order` is what
    // puts reading order and visual order in disagreement.
    expect(zones().map((zone) => zone.className)).toEqual([
      "page-zones-main",
      "page-zones-rail-column",
      "page-zones-aside-column",
    ]);
  });
});
