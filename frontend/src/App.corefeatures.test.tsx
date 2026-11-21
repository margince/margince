// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ScreenView } from "./App.views";
import { CREATE_ID } from "./app/nav";
import { LocaleProvider } from "./i18n";
import { en } from "./i18n/en";
import type { CoreFeature } from "./lib/corefeatures";
import { meFixture, READS_RECORDS } from "./testing/mefixture";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it.each<CoreFeature>([
  "home",
  "contacts",
  "companies",
  "leads",
  "deals",
  "projects",
  "filters",
  "analytics",
])("a direct %s address never mounts the disabled page", async (feature) => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    ["me"],
    meFixture({ settingsAvailability: { disabled_features: [feature] } }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ScreenView screen={feature} id="saved-record" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText(en["shell.unknownAddress"])).toBeTruthy();
  expect(fetcher).not.toHaveBeenCalled();
});

it.each<CoreFeature>(["contacts", "companies", "leads", "deals", "projects"])(
  "a %s create address never mounts the form of a disabled page",
  async (feature) => {
    const fetcher = vi.fn();
    vi.stubGlobal("fetch", fetcher);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(
      ["me"],
      meFixture({ settingsAvailability: { disabled_features: [feature] } }),
    );
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ScreenView screen={feature} id={CREATE_ID} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText(en["shell.unknownAddress"])).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fetcher).not.toHaveBeenCalled();
  },
);

it("leads an address no page answers to the first page this installation keeps, not to a Home it turned off", async () => {
  vi.stubGlobal("fetch", vi.fn());
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(
    ["me"],
    meFixture({
      allow: READS_RECORDS,
      settingsAvailability: { disabled_features: ["home"] },
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ScreenView screen="home" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(await screen.findByText(en["shell.unknownAddress"])).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Go to Home" })).toBeNull();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: `Go to ${en["nav.contacts"]}` }));
  expect(window.location.hash).toBe("#/contacts");
});
