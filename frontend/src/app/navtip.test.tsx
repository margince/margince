// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NavTip } from "./navtip";

afterEach(cleanup);

function railMove(app: Element, type: string, propertyName = "--railW") {
  act(() => {
    // happy-dom's TransitionEvent drops `propertyName` from its init.
    app.dispatchEvent(Object.assign(new Event(type), { propertyName }));
  });
}

describe("a rail tip while the rail's edge moves", () => {
  it("waits out the move and stands at the edge it settles on", () => {
    const { container } = render(
      <div className="app">
        <button type="button">
          <NavTip>Expand sidebar</NavTip>
        </button>
      </div>,
    );
    const app = container.querySelector(".app") ?? container;
    const tip = screen.getByRole("tooltip", { hidden: true });

    railMove(app, "transitionrun");
    expect(tip.style.visibility).toBe("hidden");

    railMove(app, "transitionend", "opacity");
    expect(tip.style.visibility).toBe("hidden");

    railMove(app, "transitionend");
    expect(tip.style.visibility).toBe("");
  });
});
