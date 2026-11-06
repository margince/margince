/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { Suspense } from "react";
import { afterEach, expect, it, vi } from "vitest";

async function freshLocale() {
  vi.resetModules();
  const catalogs = await import("../i18n/catalogs");
  const words = await import("../i18n");
  const views = await import("../i18n/lazy");
  return { ...catalogs, ...words, ...views };
}

afterEach(cleanup);

it("waits for the complete language before a lazy view renders", async () => {
  const { loadEssentialLocale, loadedCatalogs, LocaleProvider, lazy } =
    await freshLocale();
  await loadEssentialLocale("de");
  expect(loadedCatalogs.has("de")).toBe(false);
  const View = lazy(async () => ({
    default: () => {
      expect(loadedCatalogs.has("de")).toBe(true);
      return <p>Complete view</p>;
    },
  }));
  await act(async () => {
    render(
      <LocaleProvider initial="de" initialEssential>
        <Suspense fallback={<p>Waiting</p>}>
          <View />
        </Suspense>
      </LocaleProvider>,
    );
  });
  expect(await screen.findByText("Complete view")).toBeTruthy();
});

it("loads complete copy for a language change from an initial dictionary", async () => {
  const { useLocale, useT, LocaleProvider, loadedCatalogs } =
    await freshLocale();
  function Pick() {
    const { locale, setLocale } = useLocale();
    const t = useT();
    return (
      <button type="button" onClick={() => setLocale("vi")}>
        {locale}: {t("common.loading")}
      </button>
    );
  }
  await act(async () => {
    render(
      <LocaleProvider initial="en" initialEssential>
        <Pick />
      </LocaleProvider>,
    );
  });
  const button = await screen.findByRole("button");
  await act(async () => {
    fireEvent.click(button);
  });
  expect(
    await screen.findByRole("button", { name: "vi: Đang tải…" }),
  ).toBeTruthy();
  expect(loadedCatalogs.has("vi")).toBe(true);
});

it("keeps route children behind the complete catalog boundary", async () => {
  const { CompleteLocale, LocaleProvider, loadedCatalogs } =
    await freshLocale();
  const rendered = vi.fn();
  function View() {
    rendered();
    expect(loadedCatalogs.has("en")).toBe(true);
    return <p>Route ready</p>;
  }
  await act(async () => {
    render(
      <LocaleProvider initial="en" initialEssential>
        <Suspense fallback={null}>
          <CompleteLocale>
            <View />
          </CompleteLocale>
        </Suspense>
      </LocaleProvider>,
    );
  });
  expect(await screen.findByText("Route ready")).toBeTruthy();
  expect(rendered).toHaveBeenCalled();
});

it("retries a dictionary failure without remounting a cached lazy module", async () => {
  const { LocaleProvider, loadEssentialLocale, lazy } = await freshLocale();
  await loadEssentialLocale("en");
  const { AppErrorBoundary } = await import("../app/errorboundary");
  const reported = vi
    .spyOn(console, "error")
    .mockImplementation(() => undefined);
  vi.doMock("../i18n/en", () => {
    throw new Error("Temporary dictionary failure");
  });
  const load = vi.fn(async () => ({ default: () => <p>Recovered view</p> }));
  const View = lazy(load);
  try {
    await act(async () => {
      render(
        <LocaleProvider initial="en" initialEssential>
          <AppErrorBoundary>
            <Suspense fallback={null}>
              <View />
            </Suspense>
          </AppErrorBoundary>
        </LocaleProvider>,
      );
    });
    const retry = await screen.findByRole("button", { name: "Retry" });
    vi.doUnmock("../i18n/en");
    await act(async () => {
      fireEvent.click(retry);
    });
    expect(await screen.findByText("Recovered view")).toBeTruthy();
    expect(load).toHaveBeenCalledTimes(1);
  } finally {
    vi.doUnmock("../i18n/en");
    reported.mockRestore();
  }
});
