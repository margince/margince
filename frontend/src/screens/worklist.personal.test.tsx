/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { day, renderWorklist, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("does not fetch team oversight underneath an empty personal queue for a manager", async () => {
  stub(day({ scope: "mine", scope_options: ["mine", "team", "all"] }));
  renderWorklist();
  await screen.findByText("Nothing is waiting on you.");
  const urls = vi
    .mocked(fetch)
    .mock.calls.map(([input]) =>
      String(input instanceof Request ? input.url : input),
    );
  expect(
    urls.some((url) => /worklist\/(team|exceptions|hidden)/.test(url)),
  ).toBe(false);
});
