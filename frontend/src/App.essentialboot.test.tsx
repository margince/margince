/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Locale } from "./i18n/locale";
import { memoryStorage, sessionOnlyFetch } from "./testing/appharness";

// A direct link boots on the essential catalog. The shell draws from it in
// every language, and the screen's column waits for the complete one.

const COMPLETE = ["./i18n/en", "./i18n/de", "./i18n/vi"];
let releaseComplete = () => {};

beforeEach(() => {
  vi.resetModules();
  const arrived = new Promise<void>((resolve) => {
    releaseComplete = resolve;
  });
  for (const path of COMPLETE)
    vi.doMock(path, async () => {
      await arrived;
      return vi.importActual(path);
    });
  vi.stubGlobal("localStorage", memoryStorage());
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  window.location.hash = "#/contacts/c1";
});

afterEach(() => {
  cleanup();
  releaseComplete();
  for (const path of COMPLETE) vi.doUnmock(path);
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it.each(["en", "de", "vi"] satisfies Locale[])(
  "paints the shell in %s before the complete catalog, with no raw key",
  async (locale) => {
    const { loadEssentialLocale, loadedCatalogs } = await import(
      "./i18n/catalogs"
    );
    const { LocaleProvider } = await import("./i18n");
    const { App } = await import("./App");
    const { en } =
      await vi.importActual<typeof import("./i18n/en")>("./i18n/en");
    const { [locale]: words } = await vi.importActual<
      Record<Locale, Record<string, string>>
    >(`./i18n/${locale}`);
    await loadEssentialLocale(locale);
    await act(async () => {
      render(
        <QueryClientProvider
          client={
            new QueryClient({ defaultOptions: { queries: { retry: false } } })
          }
        >
          <LocaleProvider initial={locale}>
            <App />
          </LocaleProvider>
        </QueryClientProvider>,
      );
    });
    expect(
      (await screen.findAllByRole("navigation", {}, { timeout: 15_000 }))
        .length,
    ).toBeGreaterThan(0);
    expect(await screen.findByLabelText(words["common.loading"])).toBeTruthy();
    expect(loadedCatalogs.has(locale)).toBe(false);
    const drawn = [
      document.body.textContent ?? "",
      ...[...document.querySelectorAll("[aria-label], [title]")].flatMap(
        (element) => [
          element.getAttribute("aria-label") ?? "",
          element.getAttribute("title") ?? "",
        ],
      ),
    ].join("\n");
    expect(drawn).toContain(words["nav.contacts"]);
    expect(Object.keys(en).filter((key) => drawn.includes(key))).toEqual([]);
  },
);
