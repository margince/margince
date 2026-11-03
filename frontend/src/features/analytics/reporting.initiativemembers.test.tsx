/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { Members } from "./reporting.initiativegoal";
import { MemberFields } from "./reporting.initiativemembers";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const saved: Members = {
  data: [
    {
      record_type: "deal",
      record_id: "d-saved",
      label: "Fleet renewal",
      reason: "picked",
      included: true,
    },
  ],
  leads: 0,
  deals: 1,
  companies: 0,
  truncated: false,
};

/** Every URL the members step asked for, after the stub has answered it. */
function recordRequests(): string[] {
  const asked: string[] = [];
  const stub = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    asked.push(input instanceof Request ? input.url : String(input));
    return stub(input, init);
  }) as typeof fetch;
  return asked;
}

it("names a saved pick and finds a deal past any first page by asking the server", async () => {
  const user = userEvent.setup();
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /deals": () =>
      jsonResponse({
        data: [{ id: "d-found", name: "Harbour retrofit" }],
        page: { next_cursor: null, has_more: false },
      }),
  });
  const asked = recordRequests();
  const onPick = vi.fn();
  render(
    <StoryProviders>
      <MemberFields
        rule={{ product: "", industry: "", country: "" }}
        onRule={() => undefined}
        members={saved}
        pending={false}
        picked={[{ record_type: "deal", record_id: "d-saved" }]}
        onPick={onPick}
        excludedSet={new Set()}
        onToggle={() => undefined}
        includeFuture={false}
        onIncludeFuture={() => undefined}
      />
    </StoryProviders>,
  );
  const deals = screen.getByRole("group", { name: "Pick deals" });
  expect(within(deals).getByText("Fleet renewal")).toBeInTheDocument();

  await user.click(within(deals).getByRole("button", { name: /Add/ }));
  await user.type(screen.getByRole("searchbox"), "harbour");
  await user.click(
    await screen.findByRole("checkbox", { name: /Harbour retrofit/ }),
  );

  const search = new URL(
    asked.find((url) => url.includes("/deals?")) ?? "",
    "http://app.test",
  ).searchParams;
  expect(search.get("q")).toBe("harbour");
  expect(search.get("status")).toBe("open");
  expect(onPick).toHaveBeenLastCalledWith([
    { record_type: "deal", record_id: "d-saved" },
    { record_type: "deal", record_id: "d-found" },
  ]);
});
