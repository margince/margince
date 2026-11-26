/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { navigateReplacing } from "../lib/router";
import { replaceDial } from "../lib/urlstate";
import { useCommandPalette } from "./palettehotkey";

afterEach(() => {
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
});

describe("the palette over a page that changes", () => {
  it("closes once the page under it is another one", () => {
    globalThis.history.replaceState(null, "", "#/deals");
    const client = new QueryClient();
    const { result } = renderHook(
      () => useCommandPalette({ screen: "deals" }),
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    act(() => result.current.show("all"));
    act(() => replaceDial("stage", "won"));
    expect(result.current.open).toBe(true);
    act(() => navigateReplacing({ screen: "contacts" }));
    expect(result.current.open).toBe(false);
  });
});
