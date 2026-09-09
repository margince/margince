/** @vitest-environment jsdom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createQueryClient } from "../app/queryclient";
import { useCompany360 } from "./company360";
import { useDeal } from "./deals";
import { useDealStatusCard } from "./dealstatus";
import { usePerson360 } from "./person360";
import { useProject360 } from "./project360";

// A RECORD RE-READS ITSELF WHILE SOMEBODY IS LOOKING AT IT (FE-PARAM-5).
//
// The failure this covers is silent by construction: a page read once on
// arrival keeps answering from that instant, and a stale "what needs you"
// looks exactly like a current one — so nothing on screen says the task an
// agent filed two minutes ago is missing. Only a second read proves the
// cadence, and only a stopped clock proves the tab nobody is looking at is
// not polling all night.
//
// Driven through each record page's OWN read against the app's own client,
// rather than asserting the policy's predicate: what could actually go wrong
// is a page reading under a key the policy does not recognise, and a test of
// the predicate alone would agree with itself and pass straight through that.

// FE-PARAM-5's cadence, mirrored here so a case can advance past exactly one
// of them rather than a bare number that could mean anything.
const LIVE_RECORD_MS = 60_000;

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// THE APP'S OWN CLIENT. The cadence is a default of that client keyed on the
// read, so a bare QueryClient built here would prove a policy nothing ships.
let client: ReturnType<typeof createQueryClient>;

// What the tab reports, read by the library's focus manager through the
// `visibilitychange` event — which is what v5 listens on, not `focus`.
let visibility: DocumentVisibilityState = "visible";

function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/** Mounts one record read against a counted fetch stub. */
function mount(read: () => unknown) {
  const paths: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      paths.push(new URL(request.url).pathname);
      return jsonResponse({});
    }),
  );
  const { unmount } = renderHook(read, { wrapper });
  return { unmount, paths };
}

/** Lets every timer up to `ms` fire, and every promise they start settle. */
async function advance(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

beforeEach(() => {
  client = createQueryClient();
  visibility = "visible";
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => visibility,
  });
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(document, "visibilityState");
});

/**
 * Leaves the tab and comes back, `away` apart.
 *
 * Dispatched on WINDOW: v5's focus manager listens there, and the real
 * `visibilitychange` reaches it by bubbling up from the document. A synthetic
 * one dispatched on the document does not bubble, so it arrives nowhere — and
 * a case built on it reads exactly like a refetch that did not happen.
 */
async function leaveAndReturn(away: number): Promise<void> {
  visibility = "hidden";
  await act(async () => {
    window.dispatchEvent(new Event("visibilitychange"));
  });
  await advance(away);
  visibility = "visible";
  await act(async () => {
    window.dispatchEvent(new Event("visibilitychange"));
  });
  await advance(0);
}

describe("a record the reader has open", () => {
  // One case per composite read, and each names the record kind rather than
  // sharing a loop: a page that drops the options fails by name here.
  const reads: ReadonlyArray<[string, () => unknown, string]> = [
    ["a contact", () => usePerson360("p-1"), "/v1/people/p-1/360"],
    [
      "an account",
      () => useCompany360("o-1"),
      "/v1/companies/o-1/360",
    ],
    ["a project", () => useProject360("pr-1"), "/v1/projects/pr-1/360"],
    // The deal reads its RECORD live and its briefing not at all: that one is
    // model-written and rewritten whenever the deal has moved, so a cadence on
    // it would spend the workspace's AI budget on an open tab.
    ["a deal", () => useDeal("d-1"), "/v1/deals/d-1"],
  ];

  for (const [kind, read, path] of reads) {
    it(`re-reads ${kind} while it is on screen`, async () => {
      const { paths } = mount(read);
      await advance(0);
      expect(paths).toEqual([path]);

      await advance(LIVE_RECORD_MS);
      expect(paths).toEqual([path, path]);
    });
  }

  // The exception, pinned so that adding the cadence to it is a decision
  // somebody makes rather than a spread they copy. The briefing is written by
  // a model and rewritten server-side whenever the deal has moved, so a read
  // every twenty seconds is the workspace's AI budget spent on an open tab.
  it("leaves the model-written deal briefing on its one read", async () => {
    const { paths } = mount(() => useDealStatusCard("d-1"));
    await advance(0);

    await advance(LIVE_RECORD_MS * 3);
    expect(paths).toEqual(["/v1/deals/d-1/status"]);
  });

  // The return is what the minute-long cadence is priced against, so it has
  // to actually read. `refetchOnWindowFocus: true` would not: it refetches on
  // return only when the read is already stale, so a reader back inside
  // FE-PARAM-1's thirty seconds is served the cache and waits out the
  // interval — the one case the cadence was made slower on the strength of.
  it("re-reads on the way back in, inside the stale window", async () => {
    const { paths } = mount(() => usePerson360("p-1"));
    await advance(0);

    await leaveAndReturn(5_000);
    expect(paths).toHaveLength(2);
  });

  it("stops re-reading once the reader has closed it", async () => {
    // The interval belongs to the mounted query. A page left behind that went
    // on asking would be every record a rep opened all day, forever.
    const { unmount, paths } = mount(() => usePerson360("p-1"));
    await advance(0);
    unmount();

    await advance(LIVE_RECORD_MS * 3);
    expect(paths).toHaveLength(1);
  });
});
