// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { FirstRunNotice } from "./analytics.firstrun";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

function draw(deals: unknown[], allow: Record<string, string[]>) {
  const asked = { deals: false };
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /deals": () => {
      asked.deals = true;
      return jsonResponse({ data: deals, page: { next_cursor: null } });
    },
  });
  render(
    <StoryProviders>
      <FirstRunNotice />
    </StoryProviders>,
  );
  return asked;
}

it("names the first step when the installation has no deals", async () => {
  draw([], { deal: ["read"] });
  expect(await screen.findByText("No deals yet")).toBeInTheDocument();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Open Deals" }));
  expect(window.location.hash).toMatch(/^#\/deals/);
});

it("says nothing once a deal exists", async () => {
  const asked = draw([{ id: "d-1", name: "First" }], { deal: ["read"] });
  await vi.waitFor(() => expect(asked.deals).toBe(true));
  await act(async () => {});
  expect(screen.queryByText("No deals yet")).toBeNull();
});
