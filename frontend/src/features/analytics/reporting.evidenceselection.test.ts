/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useSelectionHeld } from "./reporting.evidenceselection";

describe("what a view holds open", () => {
  it("is dropped when the question changes, and does not come back with the first question", () => {
    const view = renderHook(
      ({ question }) => useSelectionHeld<string>(question),
      {
        initialProps: { question: "A" },
      },
    );
    act(() => view.result.current.open("won deals"));
    expect(view.result.current.held).toBe("won deals");

    view.rerender({ question: "B" });
    expect(view.result.current.held).toBeNull();
    view.rerender({ question: "A" });

    expect(view.result.current.held).toBeNull();
  });
});
