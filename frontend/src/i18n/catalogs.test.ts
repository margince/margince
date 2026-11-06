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
