/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useOrganization360 } from "./company360";
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
// Held per record read rather than once over the shared options, because what
// could actually go wrong is a page forgetting to ask for them: the constant
// staying correct while a screen quietly drops the spread is the regression,
// and a test of the constant alone would pass through it.

// FE-PARAM-5's cadence, mirrored here so a case can advance past exactly one
// of them rather than a bare number that could mean anything.
const LIVE_RECORD_MS = 20_000;

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

let client: QueryClient;

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
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("a record the reader has open", () => {
  // One case per composite read, and each names the record kind rather than
  // sharing a loop: a page that drops the options fails by name here.
  const reads: ReadonlyArray<[string, () => unknown, string]> = [
    ["a contact", () => usePerson360("p-1"), "/v1/people/p-1/360"],
    [
      "an account",
      () => useOrganization360("o-1"),
      "/v1/organizations/o-1/360",
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
