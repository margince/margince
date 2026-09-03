// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useTagChips } from "./listquery";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// An empty vocabulary is three different answers: still loading, a workspace
// with no words, and a caller who may not read them. The REQUEST carries the
// applied tag id whichever it is, so dropping the dial while one is applied
// leaves the list narrowed with no control on screen to clear it.

function chips(vocabulary: unknown) {
  installFetchStub({ "GET /tags": () => jsonResponse(vocabulary) });
  return renderHook(() => useTagChips(), { wrapper: StoryProviders });
}

afterEach(() => {
  window.location.hash = "";
  vi.unstubAllGlobals();
});

describe("the tag filter dial", () => {
  it("offers the workspace's words", async () => {
    const { result } = chips({
      data: [{ id: "t-1", workspace_id: "w", name: "Key Account" }],
      page: { has_more: false, next_cursor: null },
    });
    await waitFor(() => expect(result.current).toHaveLength(1));
    expect(result.current[0].options[0].label).toBe("Key Account");
  });

  it("draws no dial when there is nothing to filter by", async () => {
    const { result } = chips({
      data: [],
      page: { has_more: false, next_cursor: null },
    });
    await waitFor(() => expect(result.current).toEqual([]));
  });

  // The one that matters: a saved or shared address whose filter this caller
  // cannot see the words for. The rows are still narrowed, so the dial stays.
  it("keeps the dial while a tag is narrowing the list", async () => {
    window.location.hash = "#/companies?tag_id=t-9";
    const { result } = chips({
      data: [],
      page: { has_more: false, next_cursor: null },
    });
    await waitFor(() => expect(result.current).toHaveLength(1));
  });
});
