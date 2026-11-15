// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */

import { afterEach, describe, expect, it } from "vitest";
import { POINTER_OVER_ATTRIBUTE, startPointerTarget } from "./pointertarget";

const marked = () =>
  Array.from(document.querySelectorAll(`[${POINTER_OVER_ATTRIBUTE}]`)).map(
    (element) => element.id,
  );

function enter(target: Element) {
  target.dispatchEvent(new Event("pointerover", { bubbles: true }));
}

function leaveWindow(from: Element) {
  const event = new MouseEvent("pointerout", {
    bubbles: true,
    relatedTarget: null,
  });
  from.dispatchEvent(event);
}

describe("startPointerTarget", () => {
  let stop = () => {};
  afterEach(() => {
    stop();
    document.body.innerHTML = "";
  });

  it("marks only the innermost element the pointer is over", () => {
    document.body.innerHTML =
      '<div id="board"><section id="stage"><a id="card">x</a></section></div>';
    stop = startPointerTarget();
    const stage = document.getElementById("stage");
    const card = document.getElementById("card");
    if (!stage || !card) throw new Error("fixture did not render");

    enter(card);
    expect(marked()).toEqual(["card"]);
    enter(stage);
    expect(marked()).toEqual(["stage"]);
  });

  it("clears the mark when the pointer leaves the window, and when stopped", () => {
    document.body.innerHTML = '<div id="pane"><p id="line">x</p></div>';
    stop = startPointerTarget();
    const pane = document.getElementById("pane");
    const line = document.getElementById("line");
    if (!pane || !line) throw new Error("fixture did not render");

    enter(line);
    line.dispatchEvent(
      new MouseEvent("pointerout", { bubbles: true, relatedTarget: pane }),
    );
    expect(marked()).toEqual(["line"]);
    leaveWindow(line);
    expect(marked()).toEqual([]);

    enter(pane);
    stop();
    expect(marked()).toEqual([]);
    enter(line);
    expect(marked()).toEqual([]);
  });
});
