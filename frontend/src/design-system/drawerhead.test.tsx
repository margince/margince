/** @vitest-environment happy-dom */

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DrawerHead } from "./drawerbands";
import { Heading } from "./heading";

afterEach(cleanup);

describe("DrawerHead", () => {
  it("draws the mark on the title's line and the rest of the head below it", () => {
    render(
      <DrawerHead mark={<span>Public sources only</span>}>
        <Heading size="medium">Research on Anna Brandt</Heading>
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
        <Heading size="medium">Map a claim</Heading>
      </DrawerHead>,
    );

    expect(screen.getByText("Map a claim").parentElement?.className).toBe(
      "drawer-head",
    );
    expect(document.querySelector(".drawer-head-line")).toBeNull();
  });
});
