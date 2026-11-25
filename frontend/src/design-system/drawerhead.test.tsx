/** @vitest-environment happy-dom */

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DrawerHead } from "./drawerbands";

afterEach(cleanup);

describe("DrawerHead", () => {
  it("draws the mark on the title's line and the rest of the head below it", () => {
    render(
      <DrawerHead mark={<span>Public sources only</span>}>
        <h2>Research on Anna Brandt</h2>
        <p>Read on 14 Nov</p>
      </DrawerHead>,
    );

    const line = screen.getByText("Research on Anna Brandt").parentElement;
    expect(line?.className).toBe("drawer-head-line");
    expect(line?.lastElementChild?.textContent).toBe("Public sources only");
    expect(screen.getByText("Read on 14 Nov").parentElement?.className).toBe(
      "drawer-head",
    );
  });

  it("keeps the title a direct child of the band without a mark", () => {
    render(
      <DrawerHead>
        <h2>Map a claim</h2>
      </DrawerHead>,
    );

    expect(screen.getByText("Map a claim").parentElement?.className).toBe(
      "drawer-head",
    );
    expect(document.querySelector(".drawer-head-line")).toBeNull();
  });
});
