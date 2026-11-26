// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HOVER_INTENT_MS, useRecordPrefetch } from "./recordprefetch";

const ID = "01a2185f-ee02-74ff-8cff-f0905876d387";

function Shell({ children }: Readonly<{ children: ReactNode }>) {
  useRecordPrefetch();
  return <>{children}</>;
}

function renderLinks(client: QueryClient) {
  return render(
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
}

function prefetchedKeys(client: QueryClient) {
  const spy = vi.spyOn(client, "prefetchQuery").mockResolvedValue();
  return () => spy.mock.calls.map(([options]) => options.queryKey);
}

let client: QueryClient;

beforeEach(() => {
  vi.useFakeTimers();
  client = new QueryClient();
  globalThis.location.hash = "#/contacts";
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("record prefetch", () => {
  it("starts the contact page's reads once a pointer rests on its link", () => {
    const keys = prefetchedKeys(client);
    const view = renderLinks(client);
    fireEvent.pointerOver(view.getByText("Alice Müller"));
    expect(keys()).toEqual([]);
    vi.advanceTimersByTime(HOVER_INTENT_MS);
    expect(keys()).toEqual([
      ["contact", ID],
      ["contact360", ID],
      ["activities", "contact", ID, "latest-inbound"],
    ]);
  });

  it("starts nothing for a pointer that passes over on its way elsewhere", () => {
    const keys = prefetchedKeys(client);
    const view = renderLinks(client);
    fireEvent.pointerOver(view.getByText("Alice Müller"));
    fireEvent.pointerOver(document.body);
    vi.advanceTimersByTime(HOVER_INTENT_MS);
    expect(keys()).toEqual([]);
  });

  it("starts the reads at once on a press, without the hover wait", () => {
    const keys = prefetchedKeys(client);
    const view = renderLinks(client);
    fireEvent.pointerDown(view.getByText("Alice Müller"));
    expect(keys()).toHaveLength(3);
  });

  it("leaves a create form, another record kind and the open page alone", () => {
    const keys = prefetchedKeys(client);
    const view = renderLinks(client);
    fireEvent.pointerDown(view.getByText("New contact"));
    fireEvent.pointerDown(view.getByText("A deal"));
    globalThis.location.hash = `#/contacts/${ID}`;
    fireEvent.pointerDown(view.getByText("Alice Müller"));
    expect(keys()).toEqual([]);
  });
});
