/** @vitest-environment happy-dom */
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { useRoute } from "./router";

afterEach(() => {
  cleanup();
  history.replaceState(null, "", "#/home");
});
it("canonicalizes legacy Home links without adding history or losing queue filters", async () => {
  history.replaceState(
    { previous: "record" },
    "",
    "#/brief?queue=1&owner=me&filter=urgent&view=weekly",
  );
  const length = history.length;
  const { result } = renderHook(useRoute);
  await waitFor(() => expect(location.hash).toMatch(/^#\/home\?/));
  expect(result.current.screen).toBe("home");
  const params = new URLSearchParams(location.hash.split("?")[1]);
  expect(Object.fromEntries(params)).toEqual({
    queue: "1",
    owner: "me",
    filter: "urgent",
    view: "weekly",
  });
  expect(history.length).toBe(length);
  expect(history.state).toMatchObject({ previous: "record" });
});

// A path this build does not serve reads as the not-found page.
//
// The web tier answers any path with the shell, and the router reads only the
// fragment. So `/settings` used to open Home and leave the address bar on a
// path nothing resolved, with later navigation stacking onto it.
it("calls a path this build does not serve the not-found page", async () => {
  history.replaceState(null, "", "/settings");
  const { result, rerender } = renderHook(() => useRoute());
  expect(result.current.screen).toBe("not-found");

  // And keeps saying so once the bar has been moved onto the base. Read from
  // the address alone, the page would answer not-found, then become Home on
  // the next render, which tells the reader their address was fine.
  //
  // Re-rendered here because the move fires no navigation event. Without it
  // the first answer would stand whether or not it was read again.
  await waitFor(() => expect(location.pathname).toBe("/"));
  rerender();
  expect(result.current.screen).toBe("not-found");
});

// A reader can leave the not-found page by a link on it.
//
// The rail's links change only the fragment. Asked again on every read, the
// unserved path kept answering not-found and nothing on the page moved the
// reader off it.
it("lets a link on the not-found page reach a screen", async () => {
  history.replaceState(null, "", "/settings");
  const { result } = renderHook(() => useRoute());
  expect(result.current.screen).toBe("not-found");

  // What a rail link does: the fragment changes, the path does not.
  history.replaceState(null, "", `${location.pathname}#/home`);
  globalThis.dispatchEvent(new HashChangeEvent("hashchange"));

  await waitFor(() => expect(result.current.screen).toBe("home"));
});

// The address bar is moved to a path the build serves.
//
// So a reload of what the reader is now looking at answers the same page.
it("moves an unserved address onto the base, keeping the fragment", async () => {
  history.replaceState(null, "", "/settings?keep=1#/deals");
  renderHook(() => useRoute());
  await waitFor(() => expect(location.pathname).toBe("/"));
  expect(location.search).toBe("?keep=1");
  expect(location.hash).toBe("#/deals");
});

// And the base itself is not an unknown page, however it is spelled.
it("still answers the base path from its fragment", () => {
  for (const address of ["/", "/#/home", "/index.html#/home"]) {
    history.replaceState(null, "", address);
    const { result } = renderHook(() => useRoute());
    expect(result.current.screen, address).toBe("home");
    cleanup();
  }
});
