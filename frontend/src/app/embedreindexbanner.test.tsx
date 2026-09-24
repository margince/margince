/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { EmbedReindexBanner } from "./embedreindexbanner";
import { type GrantSpec, meFixture } from "./mefixture";

// The banner's status read is embedding_reindex:read server-side, granted to
// admin and ops alone — so the fixtures name that grant rather than the roles
// that happen to hold it today.
const REINDEX_READER: GrantSpec = { embedding_reindex: ["read"] };

const MISMATCH = {
  configured_identity: "anthropic/voyage-3@1024",
  populated_identity: "anthropic/voyage-2@1024",
  reindex_needed: true,
  entities_pending: 42,
};

function mount(
  allow: GrantSpec,
  status: {
    configured_identity: string;
    populated_identity: string;
    reindex_needed: boolean;
    entities_pending: number;
    status?: string;
  },
  me: {
    settingsAvailability?: NonNullable<
      Parameters<typeof meFixture>[0]
    >["settingsAvailability"];
    pending?: true;
  } = {},
) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test",
    ).pathname;
    if (path.endsWith("/me")) {
      if (me.pending) {
        return new Promise<Response>(() => {});
      }
      return new Response(
        JSON.stringify(
          meFixture({ allow, settingsAvailability: me.settingsAvailability }),
        ),
        { headers: { "Content-Type": "application/json" } },
      );
    }
    return new Response(
      JSON.stringify({
        status: "idle",
        per_workspace: [],
        ...status,
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <EmbedReindexBanner />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { fetchMock, client };
}

function statusRequests(fetchMock: ReturnType<typeof vi.fn>): number {
  return fetchMock.mock.calls.filter(([input]) =>
    String(input instanceof Request ? input.url : input).includes(
      "/embeddings/reindex/status",
    ),
  ).length;
}

// Settled means /me answered AND the render that reads it committed, so an
// observer it enabled would already have issued its request.
async function settleMe(client: QueryClient) {
  await waitFor(() =>
    expect(client.getQueryState(["me"])?.status).toBe("success"),
  );
  await act(async () => {});
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it('shows "Reindex needed" for an admin when the embed binding changed', async () => {
  mount(REINDEX_READER, {
    configured_identity: "anthropic/voyage-3@1024",
    populated_identity: "anthropic/voyage-2@1024",
    reindex_needed: true,
    entities_pending: 42,
  });
  expect(await screen.findByText("Reindex needed")).toBeTruthy();
});

it('shows "Reindex needed" for ops too', async () => {
  mount(REINDEX_READER, {
    configured_identity: "anthropic/voyage-3@1024",
    populated_identity: "anthropic/voyage-2@1024",
    reindex_needed: true,
    entities_pending: 42,
  });
  expect(await screen.findByText("Reindex needed")).toBeTruthy();
});

it("renders nothing without the read grant, even when the binding changed", async () => {
  // The GRANT is what denies here, not the role or the seat — the fixture below
  // holds a full seat and says nothing about roles. A principal with nothing
  // actionable on this surface never even probes the status read.
  const { fetchMock } = mount(
    { embedding_reindex: [] },
    {
      configured_identity: "anthropic/voyage-3@1024",
      populated_identity: "anthropic/voyage-2@1024",
      reindex_needed: true,
      entities_pending: 42,
    },
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(
    fetchMock.mock.calls.some(([input]) =>
      String(input).includes("/embeddings/reindex/status"),
    ),
  ).toBe(false);
  expect(screen.queryByText("Reindex needed")).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();
});

it("renders nothing for identity-matched drift, even with reindex_needed true and entities pending", async () => {
  // ADR-0069 §3a: matched identities + pending entities means the bus lost
  // embed events and the worker drift sweep is healing them — nothing here
  // asks a human to act, so keying off reindex_needed would wrongly fire.
  const { fetchMock } = mount(REINDEX_READER, {
    configured_identity: "anthropic/voyage-3@1024",
    populated_identity: "anthropic/voyage-3@1024",
    reindex_needed: true,
    entities_pending: 42,
  });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(screen.queryByText("Reindex needed")).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();
});

it("keeps showing during status=reembedding — a stuck marker must stay visible", async () => {
  // Deliberately NOT suppressed while a rebuild runs: a drift-cancelled
  // job leaves the marker at "reembedding" with no live worker, and
  // hiding the banner there would bury the one state that needs the
  // settings card's recovery affordance (SEARCH-AC-13: mismatch alone).
  mount(REINDEX_READER, {
    configured_identity: "anthropic/voyage-3@1024",
    populated_identity: "anthropic/voyage-2@1024",
    reindex_needed: true,
    entities_pending: 42,
    status: "reembedding",
  });
  expect(await screen.findByText("Reindex needed")).toBeTruthy();
});

it("renders nothing when the store is current", async () => {
  const { fetchMock } = mount(REINDEX_READER, {
    configured_identity: "anthropic/voyage-3@1024",
    populated_identity: "anthropic/voyage-3@1024",
    reindex_needed: false,
    entities_pending: 0,
  });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(screen.queryByText("Reindex needed")).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();
});

it("renders nothing while the status probe is pending or errors", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const path = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test",
    ).pathname;
    if (path.endsWith("/me")) {
      return new Response(
        JSON.stringify(meFixture({ allow: REINDEX_READER })),
        {
          headers: { "Content-Type": "application/json" },
        },
      );
    }
    return new Response(null, { status: 500 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <EmbedReindexBanner />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(screen.queryByText("Reindex needed")).toBeNull();
});

it("asks for the status on a bound lane and the read grant, and draws the mismatch", async () => {
  const { fetchMock } = mount(REINDEX_READER, MISMATCH, {
    settingsAvailability: { embedding_reindex: true },
  });
  expect(await screen.findByText("Reindex needed")).toBeTruthy();
  expect(statusRequests(fetchMock)).toBe(1);
});

it("asks nothing while /me has not answered", async () => {
  const { fetchMock } = mount(REINDEX_READER, MISMATCH, { pending: true });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  await act(async () => {});
  expect(statusRequests(fetchMock)).toBe(0);
  expect(screen.queryByText("Reindex needed")).toBeNull();
});

// No embeddings model bound: the status read answers 501 there, so a grant
// holder asking for it would log a refusal on every shell route.
it("asks nothing when the installation binds no embeddings model", async () => {
  const { fetchMock, client } = mount(REINDEX_READER, MISMATCH, {
    settingsAvailability: { embedding_reindex: false },
  });
  await settleMe(client);
  expect(statusRequests(fetchMock)).toBe(0);
  expect(screen.queryByText("Reindex needed")).toBeNull();
});

// A /me older than the field says nothing about the lane, which is not a yes.
it("asks nothing when /me carries no settings availability at all", async () => {
  const { fetchMock, client } = mount(REINDEX_READER, MISMATCH, {
    settingsAvailability: null,
  });
  await settleMe(client);
  expect(statusRequests(fetchMock)).toBe(0);
  expect(screen.queryByText("Reindex needed")).toBeNull();
});
