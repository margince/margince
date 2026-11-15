/** @vitest-environment happy-dom */
import { useQuery } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  renderHook,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useEmployerSummary } from "../components/contactemployersummary";
import { useEntityName } from "../components/entityref.queries";
import { CompanyScreen } from "../features/companies/companies";
import { useCompany360 } from "../features/companies/company360";
import { LeadScreen } from "../features/leads/leads";
import { useComposeRecords } from "../features/mail/composefiling";
import { useProject360 } from "../features/projects/project360";
import { useProjectRecord } from "../features/projects/projectrecord";
import { connectivityNow } from "../lib/connectivity";
import {
  contact360Query,
  contactQuery,
  relatedContactsQuery,
} from "../lib/contact.queries";
import { useDeal } from "../lib/dealread";
import { cancellationClient, type HeldRead, holdReads } from "./cancellation";

let held: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of held) read.answer({});
  });
  vi.restoreAllMocks();
});

it.each([
  { path: "/v1/contacts/c-1", useRead: () => useQuery(contactQuery("c-1")) },
  {
    path: "/v1/contacts/c-1/360",
    useRead: () => useQuery(contact360Query("c-1")),
  },
  {
    path: "/v1/contacts",
    useRead: () => useQuery(relatedContactsQuery("c-1")),
  },
  { path: "/v1/deals/d-1", useRead: () => useDeal("d-1") },
  { path: "/v1/projects/p-1", useRead: () => useProjectRecord("p-1") },
  { path: "/v1/projects/p-1/360", useRead: () => useProject360("p-1") },
  { path: "/v1/companies/c-1/360", useRead: () => useCompany360("c-1") },
  { path: "/v1/companies/c-1", useRead: () => useEmployerSummary("c-1") },
  {
    path: "/v1/deals/d-1",
    useRead: () =>
      useComposeRecords({
        open: false,
        isChannelReply: true,
        entityType: "deal",
        entityId: "d-1",
        recipientId: "",
        answering: undefined,
        subject: "",
        setSubject: () => {},
        addressed: [],
      }),
  },
])(
  "aborts $path at the HTTP boundary on unmount",
  async ({ path, useRead }) => {
    held = holdReads((url) => url.pathname === path);
    const { client, wrapper } = cancellationClient();
    const view = renderHook(
      () => {
        useRead();
      },
      { wrapper },
    );
    await waitFor(() => expect(held).toHaveLength(1));
    view.unmount();
    expect(held[0]?.request.signal.aborted).toBe(true);
    client.clear();
  },
);

it.each([
  { path: "/v1/companies/c-1", screen: <CompanyScreen id="c-1" /> },
  { path: "/v1/leads/l-1", screen: <LeadScreen id="l-1" /> },
])("aborts the screen's own $path record read", async ({ path, screen }) => {
  held = holdReads(
    (url) => url.pathname === path || url.pathname === `${path}/360`,
  );
  const { client, wrapper } = cancellationClient();
  const view = render(screen, { wrapper });
  await waitFor(() =>
    expect(
      held.some((read) => new URL(read.request.url).pathname === path),
    ).toBe(true),
  );
  view.unmount();
  expect(held.every((read) => read.request.signal.aborted)).toBe(true);
  client.clear();
});

it("cancels the old record on an id change and ignores its late answer", async () => {
  held = holdReads((url) => url.pathname.startsWith("/v1/contacts/"));
  const { client, wrapper } = cancellationClient();
  const view = renderHook(({ id }) => useQuery(contactQuery(id)), {
    wrapper,
    initialProps: { id: "first" },
  });
  await waitFor(() => expect(held).toHaveLength(1));
  view.rerender({ id: "second" });
  await waitFor(() => expect(held).toHaveLength(2));
  expect(held[0]?.request.signal.aborted).toBe(true);
  await act(async () => {
    held[0]?.answer({ id: "first" });
    held[1]?.answer({ id: "second" });
  });
  await waitFor(() => expect(view.result.current.data?.id).toBe("second"));
  expect(client.getQueryData(["contact", "first"])).toBeUndefined();
  view.unmount();
  client.clear();
});

it("keeps a shared record request alive until its last observer leaves", async () => {
  held = holdReads((url) => url.pathname === "/v1/contacts/c-1");
  const { client, wrapper } = cancellationClient();
  const first = renderHook(() => useQuery(contactQuery("c-1")), { wrapper });
  const second = renderHook(() => useQuery(contactQuery("c-1")), { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  first.unmount();
  expect(held[0]?.request.signal.aborted).toBe(false);
  await act(async () => {
    held[0]?.answer({ id: "c-1" });
  });
  await waitFor(() => expect(second.result.current.data?.id).toBe("c-1"));
  second.unmount();
  client.clear();
});

it("does not request absent ids or disabled composites", async () => {
  held = holdReads(() => true);
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => ({
      contact: useQuery(contactQuery(undefined)),
      related: useQuery(relatedContactsQuery(undefined)),
      project: useProjectRecord(undefined),
      disabledProject: useProjectRecord("p-1", false),
      company: useCompany360("c-1", false),
      unnamed: useEntityName("lead", undefined),
    }),
    { wrapper },
  );
  await act(async () => {});
  expect(held).toHaveLength(0);
  expect(view.result.current.project.settled).toBe(false);
  view.unmount();
  client.clear();
});

it.each([403, 404])(
  "preserves the employer's settled absence for HTTP %s",
  async (status) => {
    held = holdReads((url) => url.pathname === "/v1/companies/c-1");
    const { client, wrapper } = cancellationClient();
    const view = renderHook(() => useEmployerSummary("c-1"), { wrapper });
    await waitFor(() => expect(held).toHaveLength(1));
    await act(async () => {
      held[0]?.answer({ title: "Unavailable", status }, status);
    });
    await waitFor(() => expect(view.result.current.isSuccess).toBe(true));
    expect(view.result.current.data).toBeNull();
    view.unmount();
    client.clear();
  },
);

it("keeps an HTTP failure as an error rather than a cancelled or absent record", async () => {
  held = holdReads((url) => url.pathname === "/v1/contacts/c-1");
  const { client, wrapper } = cancellationClient();
  const view = renderHook(() => useQuery(contactQuery("c-1")), { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  await act(async () => {
    held[0]?.answer({ title: "Failed", status: 500 }, 500);
  });
  await waitFor(() => expect(view.result.current.isError).toBe(true));
  expect(view.result.current.data).toBeUndefined();
  view.unmount();
  client.clear();
});

it.each([403, 404])(
  "preserves the project's settled absence for HTTP %s",
  async (status) => {
    held = holdReads((url) => url.pathname === "/v1/projects/p-1");
    const { client, wrapper } = cancellationClient();
    const view = renderHook(() => useProjectRecord("p-1"), { wrapper });
    await waitFor(() => expect(held).toHaveLength(1));
    await act(async () => {
      held[0]?.answer({ title: "Unavailable", status }, status);
    });
    await waitFor(() => expect(view.result.current.settled).toBe(true));
    expect(view.result.current.project).toBeNull();
    view.unmount();
    client.clear();
  },
);

it("treats an aborted HTTP request as cancellation without reporting a connection failure", async () => {
  let request: Request | undefined;
  vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
    request = new Request(input, init);
    const signal = request.signal;
    return new Promise<Response>((_resolve, reject) => {
      signal.addEventListener("abort", () => reject(signal.reason), {
        once: true,
      });
    });
  });
  const before = connectivityNow();
  const { client, wrapper } = cancellationClient();
  const view = renderHook(() => useQuery(contactQuery("c-1")), { wrapper });
  await waitFor(() => expect(request).toBeDefined());
  await act(async () => {
    view.unmount();
  });
  expect(request?.signal.aborted).toBe(true);
  expect(connectivityNow()).toBe(before);
  expect(client.getQueryState(["contact", "c-1"])?.error).toBeNull();
  client.clear();
});
