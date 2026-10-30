/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { AiReveal, useArrivalCue } from "./aireveal";

afterEach(cleanup);

function Answer({ waiting }: Readonly<{ waiting: boolean }>) {
  const cue = useArrivalCue(waiting);
  return waiting ? (
    <p>Reading.</p>
  ) : (
    <AiReveal cue={cue}>
      <p>The answer.</p>
    </AiReveal>
  );
}

function block(): HTMLElement {
  const wrapper = screen.getByText("The answer.").parentElement;
  if (!wrapper) {
    throw new Error("the answer has no wrapper");
  }
  return wrapper;
}

describe("AiReveal", () => {
  it("stands still for an answer that was there when the surface opened", () => {
    render(<Answer waiting={false} />);
    expect(block().hasAttribute("data-ai-reveal")).toBe(false);
  });

  it("plays when an answer arrives after a wait the surface saw", () => {
    const { rerender } = render(<Answer waiting />);
    rerender(<Answer waiting={false} />);
    expect(block().getAttribute("data-ai-reveal")).toBe("run");
  });

  it("plays again for the next answer, and ends with the colour", () => {
    const { rerender } = render(<Answer waiting />);
    rerender(<Answer waiting={false} />);
    const first = block();
    // The wipe ends first; the arrival is not over until the colour has drained.
    fireEvent.animationEnd(first, { animationName: "aireveal-wipe" });
    expect(first.getAttribute("data-ai-reveal")).toBe("run");
    fireEvent.animationEnd(first, { animationName: "aireveal-color" });
    expect(first.hasAttribute("data-ai-reveal")).toBe(false);

    rerender(<Answer waiting />);
    rerender(<Answer waiting={false} />);
    expect(block().getAttribute("data-ai-reveal")).toBe("run");
  });
});
