// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { LinkedInNetworkPanel } from "./linkedin-import";

// The member's own control over whether colleagues see their reach. On by
// default, so the control has to show the on state to someone who has never
// opened it, or it reads as already withheld.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderCard(account: Record<string, unknown>): Array<unknown> {
  const saved: Array<unknown> = [];
  // The stub holds the row the way the server does, so the refetch the save
  // triggers reads back what was written. Returning the original answer would
  // leave the control showing the state the member just left.
  let stored = { ...account };
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /me/linkedin-account": () => jsonResponse(stored),
    "GET /me/linkedin-connections": () =>
      jsonResponse({ colleagues: [], total: 0 }),
    "GET /me/linkedin-reach": () =>
      jsonResponse({
        accounts: [],
        accounts_total: 0,
        unresolved_connections: 0,
      }),
    // A write handler is passed the parsed body, not the request.
    "PUT /me/linkedin-account": (body: unknown) => {
      saved.push(body);
      stored = { ...stored, ...(body as Record<string, unknown>) };
      return jsonResponse(stored);
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <LinkedInNetworkPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return saved;
}

it("shows the reach setting on to a member who has never opened it", async () => {
  renderCard({ connected: true, reach_visible_to_team: true });

  const toggle = await screen.findByRole("switch", { name: /reach/i });
  await waitFor(() => expect(toggle).toBeChecked());
});

it("shows the reach setting off to a member who withheld it", async () => {
  renderCard({ connected: true, reach_visible_to_team: false });

  const toggle = await screen.findByRole("switch", { name: /reach/i });
  await waitFor(() => expect(toggle).not.toBeChecked());
});

// The save carries the answer and nothing else. Sending a profile URL it did
// not ask about would clear the member's own, which is a separate field.
it("saves only the reach answer when the member switches it off", async () => {
  const saved = renderCard({
    connected: true,
    profile_url: "https://www.linkedin.com/in/lars",
    reach_visible_to_team: true,
  });

  const toggle = await screen.findByRole("switch", { name: /reach/i });
  // Clicked once the read has landed. A click on the control's default would
  // save the answer the member already had.
  await waitFor(() => expect(toggle).toBeChecked());
  await userEvent.click(toggle);

  await waitFor(() => expect(saved).toHaveLength(1));
  expect(saved[0]).toEqual({ reach_visible_to_team: false });
  // Still off once the refetch has landed. A control that springs back tells
  // the member their choice did not take.
  await waitFor(() => expect(toggle).not.toBeChecked());
});

// A failed read must not leave an actionable control showing the default. A
// member who withheld their reach would see it on, and a flip would write a
// state nobody read back.
it("refuses the switch when the reach setting could not be read", async () => {
  const saved: Array<unknown> = [];
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /me/linkedin-account": () =>
      new Response("", { status: 500, statusText: "read failed" }),
    "PUT /me/linkedin-account": (body: unknown) => {
      saved.push(body);
      return jsonResponse({});
    },
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <LinkedInNetworkPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  const toggle = await screen.findByRole("switch", { name: /reach/i });
  await waitFor(() => expect(toggle).toBeDisabled());
  await userEvent.click(toggle);
  expect(saved).toHaveLength(0);
});
