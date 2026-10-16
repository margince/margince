// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { linkedInReachFixture } from "../testing/reachfixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
} from "../testing/story-utils";
import { ConnectionOutcomesCard } from "./connection-outcomes";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const HELD: components["schemas"]["HeldThread"] = {
  thread_key: "t-1",
  status: "held",
  pending: false,
  attempts: 1,
  has_message: true,
  kind: "legal",
};

function routes(overrides: RouteMap = {}): RouteMap {
  return {
    "GET /me": () => jsonResponse(meFixture()),
    "GET /capture/senders": () => jsonResponse({ data: [] }),
    "GET /capture/contacts-awaiting-decision": () => jsonResponse({ data: [] }),
    "GET /capture/held-threads": () => jsonResponse({ data: [] }),
    "GET /me/linkedin-reach": () => jsonResponse(linkedInReachFixture()),
    ...overrides,
  };
}

function renderCard(client = new QueryClient()) {
  client.setDefaultOptions({ queries: { retry: false } });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConnectionOutcomesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

it("counts every account the network reaches, not the page the drawer lists", async () => {
  const page = linkedInReachFixture([
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a1",
      display_name: "Nordwind Logistik GmbH",
      connections: 4,
      contacts_on_file: 1,
    },
  ]);
  installFetchStub(
    routes({
      "GET /me/linkedin-reach": () =>
        jsonResponse({ ...page, accounts_total: 250 }),
    }),
  );
  renderCard();

  expect(await screen.findByText("250 companies")).toBeVisible();
});

it("keeps the waiting row on screen when its read fails, with a retry", async () => {
  let failing = true;
  installFetchStub(
    routes({
      "GET /capture/contacts-awaiting-decision": () =>
        failing
          ? jsonResponse({ title: "Internal Server Error" }, 500)
          : jsonResponse({ data: [] }),
    }),
  );
  const user = userEvent.setup();
  renderCard();

  const retry = await screen.findByRole("button", {
    name: "Retry Waiting on a decision",
  });
  expect(screen.getByText("Could not load")).toBeVisible();

  failing = false;
  await user.click(retry);
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Retry Waiting on a decision" }),
    ).not.toBeInTheDocument(),
  );
});

it("claims no figure while a list is still loading", async () => {
  installFetchStub(
    routes({ "GET /capture/held-threads": () => new Promise(() => {}) }),
  );
  renderCard();

  await waitFor(() => expect(screen.getAllByText("None yet")).toHaveLength(2));
  expect(document.querySelectorAll(".settingrow .skeleton")).toHaveLength(1);
});

it("returns focus to the card when the drawer's list empties before it closes", async () => {
  let held = [HELD];
  installFetchStub(
    routes({
      "GET /capture/held-threads": () => jsonResponse({ data: held }),
    }),
  );
  const user = userEvent.setup();
  const client = renderCard();

  await user.click(
    await screen.findByRole("button", { name: "View Held threads" }),
  );
  held = [];
  await client.invalidateQueries({ queryKey: ["held-threads"] });
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "View Held threads" }),
    ).not.toBeInTheDocument(),
  );
  await user.keyboard("{Escape}");

  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByText("What your connections found"),
    ),
  );
});

it("returns focus to the card while the drawer's list is still re-reading", async () => {
  let reads = 0;
  installFetchStub(
    routes({
      "GET /capture/held-threads": () => {
        reads += 1;
        return reads === 1
          ? jsonResponse({ data: [HELD] })
          : new Promise<Response>(() => {});
      },
    }),
  );
  const user = userEvent.setup();
  const client = renderCard();

  await user.click(
    await screen.findByRole("button", { name: "View Held threads" }),
  );
  void client.invalidateQueries({ queryKey: ["held-threads"] });
  await waitFor(() => expect(reads).toBe(2));
  await user.keyboard("{Escape}");

  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByText("What your connections found"),
    ),
  );
});
