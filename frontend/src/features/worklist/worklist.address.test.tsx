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
      result.current.setOwner("7b0c2a8e-4f1d-4c3a-9e2b-1a5d6c7e8f90");
    });
    expect(result.current.filter).toBe("all");
    expect(result.current.owner).toBe("7b0c2a8e-4f1d-4c3a-9e2b-1a5d6c7e8f90");
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

it("goes back to the scope's own queue when a scope is pressed over a named owner", () => {
  window.location.hash =
    "#/home?queue_scope=team&owner=7b0c2a8e-4f1d-4c3a-9e2b-1a5d6c7e8f90";
  const { result } = renderHook(() => useWorklistAddress(undefined, true));
  expect(result.current.owner).toBe("7b0c2a8e-4f1d-4c3a-9e2b-1a5d6c7e8f90");
  act(() => {
    result.current.setScope("mine");
  });
  expect(result.current.owner).toBe("");
  expect(result.current.scope).toBe("mine");
});

it("opens the reader's own queue for an owner the address cannot name", () => {
  window.location.hash = "#/home?queue_scope=team&owner=not-a-uuid";
  const { result } = renderHook(() => useWorklistAddress(undefined, true));
  expect(result.current.owner).toBe("");
});
