import { expect, it, vi } from "vitest";

it("loads the chosen language without loading the other dictionaries", async () => {
  vi.resetModules();
  const { catalogs, loadedCatalogs, loadLocale } = await import("./catalogs");
  expect(loadedCatalogs.size).toBe(0);
  const first = loadLocale("de");
  expect(loadLocale("de")).toBe(first);
  await first;
  expect(loadedCatalogs.has("de")).toBe(true);
  expect(loadedCatalogs.has("en")).toBe(false);
  expect(loadedCatalogs.has("vi")).toBe(false);
  expect(catalogs.de["common.loading"]).toBe("Wird geladen…");
  await loadLocale("vi");
  expect(loadedCatalogs.has("de")).toBe(true);
  expect(loadedCatalogs.has("vi")).toBe(true);
});

it("keeps readable error copy before a dictionary has loaded", async () => {
  vi.resetModules();
  const { catalogs } = await import("./catalogs");
  expect(catalogs.en["app.errorTitle"]).toBeTruthy();
  expect(catalogs.en["app.errorTitle"]).not.toBe("app.errorTitle");
  expect(catalogs.en["app.errorRetry"]).toBeTruthy();
});

it("allows Retry to start a fresh dictionary request after failure", async () => {
  vi.resetModules();
  vi.doMock("./de", () => {
    throw new Error("Temporary dictionary failure");
  });
  const { loadLocale, localeReady, resetLocaleFailures, loadedCatalogs } =
    await import("./catalogs");
  const failed = loadLocale("de");
  await expect(failed).rejects.toThrow();
  expect(() => localeReady("de")).toThrow();
  vi.doUnmock("./de");
  resetLocaleFailures();
  const retry = localeReady("de");
  expect(retry).not.toBe(failed);
  await retry;
  expect(loadedCatalogs.has("de")).toBe(true);
});

it("loads initial copy before completing only the selected dictionary", async () => {
  vi.resetModules();
  const { catalogs, loadEssentialLocale, localeLoaded, loadLocale } =
    await import("./catalogs");
  const { en } = await import("./en");
  const initial = loadEssentialLocale("en");
  expect(loadEssentialLocale("en")).toBe(initial);
  await initial;
  expect(localeLoaded("en", "essential")).toBe(true);
  expect(localeLoaded("en", "complete")).toBe(false);
  expect(Object.keys(catalogs.en).length).toBeLessThan(
    Object.keys(en).length / 2,
  );
  expect(catalogs.en["common.loading"]).toBe(en["common.loading"]);
  expect(Object.keys(catalogs.de).length).toBe(4);
  await loadLocale("en");
  expect(catalogs.en).toEqual(en);
  expect(localeLoaded("en", "complete")).toBe(true);
  expect(localeLoaded("de", "complete")).toBe(false);
});

it("keeps the complete dictionary when initial and complete downloads overlap", async () => {
  vi.resetModules();
  const { catalogs, loadEssentialLocale, loadLocale } = await import(
    "./catalogs"
  );
  const { de } = await import("./de");
  await Promise.all([loadEssentialLocale("de"), loadLocale("de")]);
  expect(catalogs.de).toEqual(de);
});
