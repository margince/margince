// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { useWorklistAddress } from "./worklist.address";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

it.each([true, false])(
  "opens the whole owner day from a filtered board (embedded=%s)",
  (embedded) => {
    window.location.hash =
      "#/worklist?filter=decisions&selected=old&queue_scope=team&scope=team";
    const { result } = renderHook(() =>
      useWorklistAddress(undefined, embedded),
    );
    act(() => {
      result.current.setFilter("all");
      result.current.setOwner("owner-a");
    });
    expect(result.current.filter).toBe("all");
    expect(result.current.owner).toBe("owner-a");
    expect(result.current.selectedId).toBe("");
    expect(result.current.scope).toBe("team");
  },
);

it("opens unassigned work without retaining a board filter", () => {
  window.location.hash = "#/home?filter=decisions&queue_scope=team";
  const { result } = renderHook(() => useWorklistAddress(undefined, true));
  act(() => {
    result.current.setFilter("all");
    result.current.setScope("unassigned");
  });
  expect(result.current.scope).toBe("unassigned");
  expect(result.current.filter).toBe("all");
});
