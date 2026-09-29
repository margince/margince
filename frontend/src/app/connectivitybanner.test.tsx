/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { type Connectivity, subscribeConnectivity } from "./connectivity";
import { ConnectivityBanner, ConnectivityNotice } from "./connectivitybanner";
import { createQueryClient } from "./queryclient";

afterEach(() => {
  cleanup();
  Reflect.deleteProperty(navigator, "onLine");
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function inEnglish(ui: ReactNode) {
  return <LocaleProvider initial="en">{ui}</LocaleProvider>;
}

function notice(state: Connectivity) {
  const view = render(inEnglish(<ConnectivityNotice state={state} />));
  const region = view.container.firstElementChild;
  if (!(region instanceof HTMLElement)) {
    throw new Error("the notice drew no region");
  }
  return {
    region,
    to: (next: Connectivity) =>
      view.rerender(inEnglish(<ConnectivityNotice state={next} />)),
  };
}

describe("the connectivity banner", () => {
  it("says nothing while Margince answers", () => {
    const { region } = notice("online");
    expect(region.textContent).toBe("");
    // The shell reserves the top bar's height only while no banner stands.
    expect(region.classList.contains("appbanner")).toBe(false);
  });

  it("tells an offline device from a server that does not answer", () => {
    const { region, to } = notice("offline");
    expect(screen.getByText(en["connectivity.offline.title"])).toBeTruthy();
    expect(screen.getByText(en["connectivity.offline.body"])).toBeTruthy();
    expect(region.classList.contains("appbanner")).toBe(true);

    to("unreachable");
    expect(screen.getByText(en["connectivity.unreachable.title"])).toBeTruthy();
    expect(screen.getByText(en["connectivity.unreachable.body"])).toBeTruthy();
    expect(screen.queryByText(en["connectivity.offline.title"])).toBeNull();
  });

  it("is a state, so it offers nothing to put it away", () => {
    notice("unreachable");
    expect(screen.queryByRole("button")).toBeNull();
  });

  // One region, mounted before the words arrive, is what a screen reader
  // reliably hears; the notice inside carries no role, so it is heard once.
  it("is announced politely, once", () => {
    const { region, to } = notice("online");
    expect(region.getAttribute("aria-live")).toBe("polite");
    to("offline");
    expect(region.textContent).toContain(en["connectivity.offline.title"]);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says the connection is back when it goes, and nothing on first draw", () => {
    const { region, to } = notice("unreachable");
    to("online");
    expect(region.textContent).toBe(en["connectivity.restored"]);
    expect(region.classList.contains("appbanner")).toBe(false);

    cleanup();
    expect(notice("online").region.textContent).toBe("");
  });

  it("follows the device as the browser reports it", () => {
    render(inEnglish(<ConnectivityBanner />));
    expect(screen.queryByText(en["connectivity.offline.title"])).toBeNull();

    act(() => {
      Object.defineProperty(navigator, "onLine", {
        configurable: true,
        value: false,
      });
      window.dispatchEvent(new Event("offline"));
    });
    expect(screen.getByText(en["connectivity.offline.title"])).toBeTruthy();
  });
});

// A content blocker refusing one analytics URL, or a firewall's bare 503 on one
// route: Margince itself answers, so the banner must not flap on every refetch.
describe("one refused path while Margince answers", () => {
  function BlockedRead() {
    const read = useQuery({
      queryKey: ["blocked-read"],
      queryFn: () => api.GET("/me"),
      retry: false,
    });
    return <p>{read.isError ? "read failed" : "reading"}</p>;
  }

  it("never shows the banner", async () => {
    vi.useFakeTimers();
    const probes = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url === "/healthz") {
          probes();
          return new Response("ok");
        }
        throw new TypeError("net::ERR_BLOCKED_BY_CLIENT");
      }),
    );
    const flips = vi.fn();
    const unsubscribe = subscribeConnectivity(flips);
    render(
      <QueryClientProvider client={createQueryClient()}>
        {inEnglish(
          <>
            <ConnectivityBanner />
            <BlockedRead />
          </>,
        )}
      </QueryClientProvider>,
    );

    for (let elapsed = 0; elapsed < 20_000; elapsed += 250) {
      await act(() => vi.advanceTimersByTimeAsync(250));
      expect(
        screen.queryByText(en["connectivity.unreachable.title"]),
      ).toBeNull();
    }
    expect(screen.getByText("read failed")).toBeTruthy();
    expect(probes).toHaveBeenCalledTimes(1);
    expect(flips).not.toHaveBeenCalled();
    unsubscribe();
  });
});
