/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { EntityKind } from "../lib/entity";
import {
  companyReferenceQuery,
  entityNameQuery,
  fetchEntityName,
  fetchEntityReference,
  useEntityName,
} from "./entityref.queries";

const KINDS: readonly EntityKind[] = [
  "contact",
  "company",
  "lead",
  "deal",
  "project",
];
const clients: QueryClient[] = [];
function session() {
  const client = new QueryClient();
  clients.push(client);
  return {
    client,
    wrapper: ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children),
  };
}
function urlOf(input: RequestInfo | URL) {
  return new URL(new Request(input).url);
}
function answer(url: URL) {
  return Response.json({
    data: url.searchParams.getAll("id").map((id) => ({
      id,
      name: `${url.searchParams.get("kind")} ${id}`,
      archived: false,
    })),
  });
}
afterEach(() => {
  cleanup();
  for (const client of clients) client.clear();
  clients.length = 0;
  vi.restoreAllMocks();
});

it.each(KINDS)(
  "batches 100 and 101 unique %s names without detail or list requests",
  async (kind) => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input) => answer(urlOf(input)));
    const { wrapper } = session();
    const ids = Array.from(
      { length: 100 },
      (_, i) => `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`,
    );
    const first = renderHook(
      () => [...ids, ...ids].map((id) => useEntityName(kind, id)),
      { wrapper },
    );
    await waitFor(() =>
      expect(first.result.current.every((value) => value.name)).toBe(true),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const extra = "00000000-0000-4000-8000-000000000100";
    const second = renderHook(
      () => [...ids, extra].map((id) => useEntityName(kind, id)),
      { wrapper },
    );
    await waitFor(() =>
      expect(second.result.current[100]?.name).toBe(`${kind} ${extra}`),
    );
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const requests = fetchMock.mock.calls.map(([input]) => urlOf(input));
    expect(requests.map((url) => url.searchParams.getAll("id").length)).toEqual(
      [100, 1],
    );
    expect(
      requests.every(
        (url) =>
          url.pathname === "/v1/records/references" &&
          url.searchParams.get("kind") === kind,
      ),
    ).toBe(true);
    expect(
      requests.every((url) =>
        [...url.searchParams.keys()].every(
          (key) => key === "kind" || key === "id",
        ),
      ),
    ).toBe(true);
  },
);

it.each(KINDS)(
  "splits a cold batch of 101 %s ids into bounded requests",
  async (kind) => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input) => answer(urlOf(input)));
    const { wrapper } = session();
    const ids = Array.from({ length: 101 }, (_, i) => `id-${i}`);
    const view = renderHook(() => ids.map((id) => useEntityName(kind, id)), {
      wrapper,
    });
    await waitFor(() =>
      expect(view.result.current.every((value) => value.name)).toBe(true),
    );
    expect(
      fetchMock.mock.calls.map(
        ([input]) => urlOf(input).searchParams.getAll("id").length,
      ),
    ).toEqual([100, 1]);
  },
);

it("separates kinds sharing an id and preserves rich company versus string cache shapes", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) => answer(urlOf(input)));
  const { client, wrapper } = session();
  const view = renderHook(
    () => KINDS.map((kind) => useEntityName(kind, "same-id")),
    { wrapper },
  );
  await waitFor(() =>
    expect(view.result.current.every((value) => value.name)).toBe(true),
  );
  expect(fetchMock).toHaveBeenCalledTimes(5);
  for (const kind of KINDS) {
    const value = client.getQueryData(
      kind === "company"
        ? companyReferenceQuery("same-id").queryKey
        : entityNameQuery(kind, "same-id").queryKey,
    );
    expect(value).toEqual(
      kind === "company"
        ? { name: "company same-id", archived: false, logo: undefined }
        : `${kind} same-id`,
    );
  }
});

it.each(KINDS)(
  "maps missing and scope-hidden %s ids to the same settled null",
  async (kind) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({ data: [] }),
    );
    const { client, wrapper } = session();
    const view = renderHook(
      () => [useEntityName(kind, "missing"), useEntityName(kind, "hidden")],
      { wrapper },
    );
    await waitFor(() =>
      expect(
        view.result.current.every((value) => value.reading === "unnamed"),
      ).toBe(true),
    );
    expect(view.result.current.map((value) => value.name)).toEqual([
      null,
      null,
    ]);
    const key =
      kind === "company"
        ? companyReferenceQuery("hidden").queryKey
        : entityNameQuery(kind, "hidden").queryKey;
    expect(client.getQueryData(key)).toBeNull();
  },
);

it.each([403, 404, 500])(
  "holds HTTP %s as a failure without per-record retries",
  async (status) => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        Response.json({ title: "Unavailable", status }, { status }),
      );
    const { wrapper } = session();
    const view = renderHook(
      () => [useEntityName("contact", "one"), useEntityName("contact", "two")],
      { wrapper },
    );
    await waitFor(() =>
      expect(
        view.result.current.every((value) => value.reading === "failed"),
      ).toBe(true),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  },
);

it("preserves successful pages when a sibling page fails", async () => {
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input) => {
      const url = urlOf(input);
      return url.searchParams.getAll("id").includes("id-100")
        ? Response.json({ title: "Forbidden", status: 403 }, { status: 403 })
        : answer(url);
    });
  const { wrapper } = session();
  const view = renderHook(
    () =>
      Array.from({ length: 101 }, (_, i) =>
        useEntityName("project", `id-${i}`),
      ),
    { wrapper },
  );
  await waitFor(() => expect(view.result.current[100]?.reading).toBe("failed"));
  expect(view.result.current[0]?.name).toBe("project id-0");
  expect(fetchMock).toHaveBeenCalledTimes(2);
});

it("returns archive and logo metadata without caching a record id inside the rich value", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      data: [
        { id: "anchor", name: null, archived: true, logo: "/logo/revision-2" },
      ],
    }),
  );
  const { client } = session();
  expect(await client.fetchQuery(companyReferenceQuery("anchor"))).toEqual({
    name: null,
    archived: true,
    logo: "/logo/revision-2",
  });
});

it.each(KINDS)(
  "retains a %s batch answer for an unmounted observer and its sibling",
  async (kind) => {
    let release!: (response: Response) => void;
    let transport: Request | undefined;
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation((input, init) => {
        transport = new Request(input, init);
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      });
    const { wrapper } = session();
    const first = renderHook(() => useEntityName(kind, "one"), { wrapper });
    const second = renderHook(() => useEntityName(kind, "two"), { wrapper });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    first.unmount();
    expect(transport?.signal.aborted).toBe(false);
    await act(async () => {
      release(
        Response.json({
          data: [
            { id: "one", name: "First", archived: false },
            { id: "two", name: "Second", archived: false },
          ],
        }),
      );
    });
    await waitFor(() => expect(second.result.current.name).toBe("Second"));
    const again = renderHook(() => useEntityName(kind, "one"), { wrapper });
    expect(again.result.current.name).toBe("First");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  },
);

it("checks a direct helper's abort before queuing and after awaiting without aborting its sibling", async () => {
  let release!: (response: Response) => void;
  let transport: Request | undefined;
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input, init) => {
      transport = new Request(input, init);
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    });
  const aborted = new AbortController();
  aborted.abort();
  await expect(
    fetchEntityName("contact", "unsent", aborted.signal),
  ).rejects.toMatchObject({ name: "AbortError" });
  expect(fetchMock).not.toHaveBeenCalled();
  const controller = new AbortController();
  const cancelled = fetchEntityReference(
    "contact",
    "one",
    controller.signal,
  ).catch((error: unknown) => error);
  const sibling = fetchEntityReference(
    "contact",
    "two",
    new AbortController().signal,
  );
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  controller.abort();
  expect(transport?.signal.aborted).toBe(false);
  release(
    Response.json({
      data: [
        { id: "one", name: "First", archived: true },
        { id: "two", name: "Second", archived: false },
      ],
    }),
  );
  expect(await cancelled).toMatchObject({ name: "AbortError" });
  expect(await sibling).toEqual({ name: "Second", archived: false });
});

it("does not let a late answer from a cleared session replace the next session's scoped answer", async () => {
  const releases: Array<(response: Response) => void> = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        releases.push(resolve);
      }),
  );
  const oldSession = session();
  const first = renderHook(() => useEntityName("deal", "same"), {
    wrapper: oldSession.wrapper,
  });
  await waitFor(() => expect(releases).toHaveLength(1));
  first.unmount();
  oldSession.client.clear();
  const nextSession = session();
  const second = renderHook(() => useEntityName("deal", "same"), {
    wrapper: nextSession.wrapper,
  });
  await waitFor(() => expect(releases).toHaveLength(2));
  await act(async () => {
    releases[1]?.(Response.json({ data: [] }));
  });
  await waitFor(() => expect(second.result.current.reading).toBe("unnamed"));
  await act(async () => {
    releases[0]?.(
      Response.json({
        data: [{ id: "same", name: "Old principal", archived: false }],
      }),
    );
  });
  expect(second.result.current.name).toBeNull();
  expect(
    nextSession.client.getQueryData(entityNameQuery("deal", "same").queryKey),
  ).toBeNull();
});

it("cancels one cache query without aborting the sibling request in its batch", async () => {
  let release!: (response: Response) => void;
  let transport: Request | undefined;
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation((input, init) => {
      transport = new Request(input, init);
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    });
  const { client, wrapper } = session();
  const first = renderHook(() => useEntityName("contact", "one"), { wrapper });
  const second = renderHook(() => useEntityName("contact", "two"), { wrapper });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  await client.cancelQueries({
    queryKey: entityNameQuery("contact", "one").queryKey,
    exact: true,
  });
  expect(transport?.signal.aborted).toBe(false);
  await act(async () => {
    release(
      Response.json({
        data: [
          { id: "one", name: "First", archived: false },
          { id: "two", name: "Second", archived: false },
        ],
      }),
    );
  });
  await waitFor(() => expect(second.result.current.name).toBe("Second"));
  expect(first.result.current.name).toBeNull();
  expect(
    client.getQueryData(entityNameQuery("contact", "one").queryKey),
  ).toBeUndefined();
});

it("returns archived nameless records through the direct helper without inventing a name", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
    Response.json({ data: [{ id: "blank", name: null, archived: true }] }),
  );
  expect(
    await fetchEntityReference("lead", "blank", new AbortController().signal),
  ).toEqual({ name: null, archived: true });
  const { client } = session();
  expect(await client.fetchQuery(entityNameQuery("lead", "blank"))).toBeNull();
});
