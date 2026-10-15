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
  expect(history.state).toEqual({ previous: "record" });
});
