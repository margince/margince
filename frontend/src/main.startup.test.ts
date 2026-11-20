/** @vitest-environment happy-dom */
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { stable } from "./format/collate";
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
  localStorage.setItem("margince.session.seen", "1");
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

it("asks nothing before mounting another route on a browser that never had a session", async () => {
  location.hash = "#/deals";
  const { loadLocale } = await import("./i18n/catalogs");
  await loadLocale("en");
  const fetch = vi.spyOn(globalThis, "fetch");
  await import("./bootstrap");
  await import("./main");
  await vi.waitFor(() => expect(createRoot).toHaveBeenCalledTimes(1));
  expect(fetch).not.toHaveBeenCalled();
});

it("starts the shell's reads and the screen's code together, before the language arrives", async () => {
  location.hash = "#/deals/d1";
  localStorage.setItem("margince.session.seen", "1");
  let releaseLanguage = () => {};
  const language = new Promise<void>((resolve) => {
    releaseLanguage = resolve;
  });
  vi.doMock("./i18n/essential/en_gen.json", async () => {
    await language;
    return vi.importActual("./i18n/essential/en_gen.json");
  });
  vi.doMock("./i18n/en", async () => {
    await language;
    return vi.importActual("./i18n/en");
  });
  const { SCREEN_CODE } = await import("./App.screencode");
  const screenCode = vi.spyOn(SCREEN_CODE, "deals");
  const paths: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation((input) => {
    paths.push(
      new URL(input instanceof Request ? input.url : String(input)).pathname,
    );
    return new Promise<Response>(() => {});
  });
  try {
    await import("./bootstrap");
    await vi.waitFor(() => expect(paths).toHaveLength(4));
    expect([...paths].sort(stable)).toEqual([
      "/v1/company/context/capabilities",
      "/v1/installation/settings",
      "/v1/me",
      "/v1/onboarding/state",
    ]);
    expect(screenCode).toHaveBeenCalledTimes(1);
    await import("./main");
    expect(createRoot).not.toHaveBeenCalled();
    releaseLanguage();
    await vi.waitFor(() => expect(createRoot).toHaveBeenCalledTimes(1));
    expect(paths).toHaveLength(4);
  } finally {
    vi.doUnmock("./i18n/essential/en_gen.json");
    vi.doUnmock("./i18n/en");
  }
});
