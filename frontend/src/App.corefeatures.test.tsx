// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ScreenView } from "./App.views";
import { LocaleProvider } from "./i18n";
import { en } from "./i18n/en";
import type { CoreFeature } from "./lib/corefeatures";
import { meFixture } from "./testing/mefixture";

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
  expect(await screen.findByText(en["shell.unknownPage"])).toBeTruthy();
  expect(fetcher).not.toHaveBeenCalled();
});
