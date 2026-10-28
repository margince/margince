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
// The web tier answers any path with the shell and the router reads only the
// fragment, so `/settings` used to open Home and leave the address bar on a
// path nothing resolved. Later navigation then stacked onto it.
it("calls a path this build does not serve the not-found page", () => {
  history.replaceState(null, "", "/settings");
  const { result } = renderHook(() => useRoute());
  expect(result.current.screen).toBe("not-found");
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
