/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
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

// The details pane folding away. What the stylesheet does with the track is a
// picture (pagezones.stories.tsx); what is spec here is the LANDMARK: a region
// a reader cannot see and cannot reach is the one state this must never rest
// in, so the aside is held exactly as long as its exit runs and no longer.
function withAside(open: boolean) {
  return (
    <PageZones
      shape="aside"
      main={<p>{WORK}</p>}
      aside={<p>what it is worth</p>}
      asideLabel={CONTEXT}
      asideOpen={open}
    />
  );
}

function asideColumn(container: HTMLElement): HTMLElement | null {
  return container.querySelector("aside.page-zones-aside-column");
}

describe("PageZones details pane", () => {
  it("draws no aside for a pane that starts folded", () => {
    const { container } = render(withAside(false));
    expect(asideColumn(container)).toBeNull();
  });

  it("keeps the same grid whether the pane is open or folded", () => {
    const { container, rerender } = render(withAside(true));
    const grid = container.firstElementChild;
    const open = grid?.className;
    rerender(withAside(false));
    // The closed state is the same template with the track at zero. A grid
    // class that changed with the pane would name a SECOND template, and two
    // templates do not interpolate — the column would jump where it travels.
    expect(container.firstElementChild?.className).toBe(open);
    expect(container.firstElementChild).toBe(grid);
  });

  it("drops the landmark in the same commit when nothing is animating", () => {
    const { container, rerender } = render(withAside(true));
    // No Web Animations API in this environment, which is the reduced-motion
    // path spelled by the runtime: a reader who asked for less motion runs no
    // transition, so there is nothing to wait for and nothing to hold.
    rerender(withAside(false));
    expect(asideColumn(container)).toBeNull();
  });
});

// The same fold with an exit actually running. happy-dom implements no Web
// Animations API, so the animation is supplied here and ENDED by the test:
// that is what puts the assertion on the transition's end rather than on a
// duration this file would then have to keep in step with the stylesheet.
describe("PageZones details pane, while its exit runs", () => {
  let endExit: () => void;
  let original: PropertyDescriptor | undefined;

  beforeEach(() => {
    let end = (): void => undefined;
    const finished = new Promise<void>((resolve) => {
      end = resolve;
    });
    endExit = () => end();
    // The two members the presence hook reads: whether the animation ENDS at
    // all — a loop inside the pane is not an exit and is never waited for —
    // and the promise that says when. A fuller fake would be asserting our own
    // idea of the API rather than the part this depends on.
    const animation = {
      finished,
      effect: { getComputedTiming: () => ({ iterations: 1 }) },
    } as unknown as Animation;
    original = Object.getOwnPropertyDescriptor(
      Element.prototype,
      "getAnimations",
    );
    Object.defineProperty(Element.prototype, "getAnimations", {
      configurable: true,
      value: () => [animation],
    });
  });

  afterEach(() => {
    if (original) {
      Object.defineProperty(Element.prototype, "getAnimations", original);
      return;
    }
    Reflect.deleteProperty(Element.prototype, "getAnimations");
  });

  it("holds the landmark inert until the exit has run, then drops it", async () => {
    const { container, rerender } = render(withAside(true));
    expect(asideColumn(container)?.hasAttribute("inert")).toBe(false);

    rerender(withAside(false));
    const leaving = asideColumn(container);
    expect(leaving).not.toBeNull();
    expect(leaving?.getAttribute("data-state")).toBe("closing");
    // Reachable by tab while it is leaving is focus dropped to the top of the
    // document the moment it goes.
    expect(leaving?.hasAttribute("inert")).toBe(true);

    endExit();
    await waitFor(() => {
      expect(asideColumn(container)).toBeNull();
    });
  });
});
