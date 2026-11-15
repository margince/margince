// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import {
  hashKey,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import type { CfObject } from "../lib/customfields.logic";
import { CATALOG_WAIT_MS, useSettledOpen } from "./create.dialog";
import { useObjectCustomFields } from "./customfields.form";
import { companyReferenceQuery } from "./entityref.queries";

const clients: QueryClient[] = [];

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  for (const client of clients) client.clear();
  clients.length = 0;
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function catalogClient() {
  let hashes = 0;
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        queryKeyHashFn: (key) => {
          hashes += 1;
          return `catalog-test:${hashKey(key)}`;
        },
      },
    },
  });
  clients.push(client);
  let release!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => response),
  );
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return {
    client,
    hashes: () => hashes,
    resetHashes: () => {
      hashes = 0;
    },
    complete: async () => {
      await act(async () => {
        release(Response.json({ data: [] }));
        await vi.advanceTimersByTimeAsync(0);
      });
    },
    mount: (catalog: CfObject | undefined) =>
      renderHook(
        ({ open }) => {
          useObjectCustomFields("contact");
          return useSettledOpen(open, catalog);
        },
        { wrapper, initialProps: { open: false } },
      ),
  };
}

it("does not rehash a large reference cache while the dialog is closed", async () => {
  const catalog = catalogClient();
  for (let index = 0; index < 200; index += 1) {
    catalog.client.setQueryData(companyReferenceQuery(`c-${index}`).queryKey, {
      name: `Company ${index}`,
      archived: false,
    });
  }
  catalog.resetHashes();

  const view = catalog.mount("contact");

  expect(view.result.current).toBe(false);
  expect(catalog.hashes()).toBeLessThan(20);
  await catalog.complete();
  expect(view.result.current).toBe(false);
});

it("waits for its first catalog read with the configured query hash", async () => {
  const catalog = catalogClient();
  const view = catalog.mount("contact");
  view.rerender({ open: true });

  expect(view.result.current).toBe(false);
  await catalog.complete();
  expect(view.result.current).toBe(true);
});

const otherCatalogs: Array<CfObject | undefined> = ["company", undefined];
it.each(otherCatalogs)(
  "does not wait for a contact catalog when its own catalog is %s",
  async (object) => {
    const catalog = catalogClient();
    const view = catalog.mount(object);
    view.rerender({ open: true });

    expect(view.result.current).toBe(true);
    await catalog.complete();
  },
);

it("opens with cached fields while their refresh is pending", async () => {
  const catalog = catalogClient();
  catalog.client.setQueryData(["custom-fields", "contact"], { data: [] });
  const view = catalog.mount("contact");
  view.rerender({ open: true });

  expect(view.result.current).toBe(true);
  await catalog.complete();
});

it("opens the core form when the first catalog read reaches its wait limit", async () => {
  const catalog = catalogClient();
  const view = catalog.mount("contact");
  view.rerender({ open: true });
  await act(() => vi.advanceTimersByTimeAsync(CATALOG_WAIT_MS - 1));
  expect(view.result.current).toBe(false);
  await act(() => vi.advanceTimersByTimeAsync(1));
  expect(view.result.current).toBe(true);
  await catalog.complete();
});
