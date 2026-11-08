/** @vitest-environment happy-dom */
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { meFixture } from "./testing/mefixture";

vi.mock("react-dom/client", () => ({
  createRoot: vi.fn(() => ({ render: vi.fn() })),
}));

beforeEach(() => {
  vi.resetModules();
  vi.clearAllMocks();
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-22T12:00:00Z"));
  localStorage.clear();
  sessionStorage.clear();
  document.body.innerHTML = '<div id="root"></div>';
});
afterEach(async () => {
  const { queryClient } = await import("./app/queryclient.instance");
  queryClient.clear();
  vi.doUnmock("./app/analytics-prefetch");
  vi.restoreAllMocks();
  vi.useRealTimers();
  location.hash = "";
});

it("registers the prepared session before mounting when the preparation chunk arrives last", async () => {
  location.hash = "#/analytics/performance";
  const { loadEssentialLocale } = await import("./i18n/catalogs");
  await loadEssentialLocale("en");
  let releaseChunk: () => void = () => {
    throw new Error("Preparation gate was not initialized");
  };
  const chunk = new Promise<void>((resolve) => {
    releaseChunk = resolve;
  });
  vi.doMock("./app/analytics-prefetch", async () => {
    await chunk;
    return vi.importActual("./app/analytics-prefetch");
  });
  let answerSession: (response: Response) => void = (_response) => {
    throw new Error("Session gate was not initialized");
  };
  const response = new Promise<Response>((resolve) => {
    answerSession = resolve;
  });
  const requests: URL[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    requests.push(
      new URL(input instanceof Request ? input.url : String(input)),
    );
    return response;
  });
  await import("./bootstrap");
  await import("./main");
  expect(createRoot).not.toHaveBeenCalled();
  expect(requests).toHaveLength(1);
  expect(requests[0]?.searchParams.get("prepare")).toBe("analytics");

  releaseChunk();
  await vi.waitFor(() => expect(createRoot).toHaveBeenCalledTimes(1));
  const { queryClient } = await import("./app/queryclient.instance");
  expect(queryClient.getQueryState(["me"])?.fetchStatus).toBe("fetching");
  expect(requests).toHaveLength(1);

  location.hash = "#/deals";
  const me = meFixture();
  answerSession(Response.json(me));
  await vi.waitFor(() => expect(queryClient.getQueryData(["me"])).toEqual(me));
});

it("mounts another route without importing Analytics preparation", async () => {
  location.hash = "#/deals";
  const { loadLocale } = await import("./i18n/catalogs");
  await loadLocale("en");
  const fetch = vi.spyOn(globalThis, "fetch");
  await import("./bootstrap");
  await import("./main");
  await vi.waitFor(() => expect(createRoot).toHaveBeenCalledTimes(1));
  expect(fetch).not.toHaveBeenCalled();
});
