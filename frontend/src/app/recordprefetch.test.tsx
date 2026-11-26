// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sessionOptions } from "../lib/session";
import { meFixture } from "../testing/mefixture";
import { createQueryClient } from "./queryclient";
import { HOVER_INTENT_MS, useRecordPrefetch } from "./recordprefetch";

const ID = "01a2185f-ee02-74ff-8cff-f0905876d387";

const READER = meFixture({ allow: { contact: ["read"] }, rowScope: "all" });

// The page's three reads are held open until the case answers them.
function server() {
  const asked: Request[] = [];
  const waiting: Array<() => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      asked.push(request);
      await new Promise<void>((resolve) => waiting.push(resolve));
      return Response.json({ data: [] });
    }),
  );
  const paths = () =>
    asked.map((request) => {
      const url = new URL(request.url);
      return url.pathname + url.search;
    });
  return {
    asked,
    paths,
    answerAll: () => {
      for (const answer of waiting.splice(0)) answer();
    },
  };
}

function Shell({ children }: Readonly<{ children: ReactNode }>) {
  useRecordPrefetch();
  return <>{children}</>;
}

function renderLinks(me = READER) {
  const client = createQueryClient();
  client.setQueryData(sessionOptions.queryKey, me);
  const view = render(
    <QueryClientProvider client={client}>
      <Shell>
        <a href={`/r/contacts/${ID}#/contacts/${ID}`}>
          <span>Alice Müller</span>
        </a>
        <a href="#/contacts/new">New contact</a>
        <a href={`#/deals/${ID}`}>A deal</a>
      </Shell>
    </QueryClientProvider>,
  );
  return { client, view };
}

function rest(element: Element) {
  vi.useFakeTimers();
  fireEvent.pointerOver(element, { pointerType: "mouse" });
  act(() => {
    vi.advanceTimersByTime(HOVER_INTENT_MS);
  });
  vi.useRealTimers();
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("record links warm the page they open", () => {
  it("starts the contact page's reads once a pointer rests on its link", async () => {
    const backend = server();
    const { view } = renderLinks();
    rest(view.getByText("Alice Müller"));

    await waitFor(() => expect(backend.asked).toHaveLength(3));
    expect(new Set(backend.paths())).toEqual(
      new Set([
        `/v1/activities/latest-inbound?entity_type=contact&entity_id=${ID}`,
        `/v1/contacts/${ID}`,
        `/v1/contacts/${ID}/360`,
      ]),
    );
  });

  it("starts nothing for a pointer that passes over on its way elsewhere", async () => {
    const backend = server();
    const { view } = renderLinks();
    vi.useFakeTimers();
    fireEvent.pointerOver(view.getByText("Alice Müller"), {
      pointerType: "mouse",
    });
    fireEvent.pointerOver(document.body, { pointerType: "mouse" });
    act(() => {
      vi.advanceTimersByTime(HOVER_INTENT_MS);
    });
    vi.useRealTimers();

    await act(() => Promise.resolve());
    expect(backend.asked).toHaveLength(0);
  });

  it("lets the reads go when the pointer leaves before the click", async () => {
    const backend = server();
    const { view } = renderLinks();
    rest(view.getByText("Alice Müller"));
    await waitFor(() => expect(backend.asked).toHaveLength(3));

    fireEvent.pointerOver(document.body, { pointerType: "mouse" });

    const row = backend.asked.find((request) =>
      request.url.endsWith(`/contacts/${ID}`),
    );
    await waitFor(() => expect(row?.signal.aborted).toBe(true));
  });

  it("keeps a clicked link's reads for the page to adopt", async () => {
    const backend = server();
    const { client, view } = renderLinks();
    const link = view.getByText("Alice Müller");
    fireEvent.pointerDown(link, { button: 0 });
    await waitFor(() => expect(backend.asked).toHaveLength(3));
    fireEvent.click(link);
    fireEvent.pointerOver(document.body, { pointerType: "mouse" });
    backend.answerAll();

    await waitFor(() =>
      expect(client.getQueryData(["contact360", ID])).toEqual({ data: [] }),
    );
    expect(backend.asked.some((request) => request.signal.aborted)).toBe(false);
  });

  it("leaves a create form, another record kind and the open page alone", async () => {
    const backend = server();
    const { view } = renderLinks();
    fireEvent.pointerDown(view.getByText("New contact"), { button: 0 });
    fireEvent.pointerDown(view.getByText("A deal"), { button: 0 });
    window.location.hash = `#/contacts/${ID}`;
    fireEvent.pointerDown(view.getByText("Alice Müller"), { button: 0 });

    await act(() => Promise.resolve());
    expect(backend.asked).toHaveLength(0);
  });

  it("starts nothing for a reader without contact read", async () => {
    const backend = server();
    const { view } = renderLinks(meFixture({ allow: { deal: ["read"] } }));
    fireEvent.pointerDown(view.getByText("Alice Müller"), { button: 0 });

    await act(() => Promise.resolve());
    expect(backend.asked).toHaveLength(0);
  });
});
