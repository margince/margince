/** @vitest-environment happy-dom */
import type { QueryClient } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import {
  armHoverIntent,
  takeHoverClock,
} from "../design-system/hoverintent-testing";
import { ContactsScreen } from "../features/contacts/contacts";
import { contactsArrivalOptions } from "../features/contacts/contacts.queries";
import { DealsScreen } from "../features/deals/deals";
import { dealsArrivalColumnOptions } from "../features/deals/deals.opening";
import {
  deal,
  render as renderDeals,
  stages,
  stubBackend,
} from "../features/deals/deals.testkit";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { sessionOptions } from "../lib/session";
import { meFixture } from "../testing/mefixture";
import { renderWith } from "../testing/shellharness";
import { navigationPrefetch } from "./navigation-prefetch";
import { createQueryClient } from "./queryclient";
import { WorkspaceRail } from "./shell";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  window.location.hash = "";
  window.localStorage.clear();
});

type Pipeline = components["schemas"]["Pipeline"];

const READER = meFixture({
  allow: { contact: ["read"], deal: ["read"] },
  rowScope: "all",
});

const OTHER_READER = {
  ...READER,
  user: { ...READER.user, id: "00000000-0000-4000-8000-000000000002" },
};

const anna = {
  id: "p-1",
  full_name: "Anna Weber",
  emails: [],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
};

// The app's own client, so a mount meets the same freshness window it would.
function signedIn(me = READER) {
  const client = createQueryClient();
  client.setQueryData(sessionOptions.queryKey, me);
  return client;
}

// A server that answers the Contacts list from a queue the case releases, and
// every other read with an empty page.
function contactsServer() {
  const asked: Request[] = [];
  const waiting: Array<() => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/contacts")) {
        asked.push(request);
        await new Promise<void>((resolve) => waiting.push(resolve));
        return Response.json({
          data: [anna],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.pathname.endsWith("/me")) return Response.json(READER);
      return Response.json({ data: [], page: { has_more: false } });
    }),
  );
  return {
    asked,
    answerAll: () => {
      for (const answer of waiting.splice(0)) answer();
    },
  };
}

function rail(client: QueryClient) {
  renderWith(client, <WorkspaceRail route={{ screen: "home" }} />);
  return screen.getByRole("link", { name: "Contacts" });
}

// Rests a mouse pointer on the link for `restMs` and moves it away, on the
// hover clock, then hands the real clock back for the reads to settle.
function pointAt(link: HTMLElement, restMs: number, leave = false) {
  takeHoverClock();
  armHoverIntent();
  fireEvent.pointerEnter(link, { pointerType: "mouse" });
  act(() => {
    vi.advanceTimersByTime(restMs);
  });
  if (leave) {
    fireEvent.pointerLeave(link, { pointerType: "mouse" });
    act(() => {
      vi.advanceTimersByTime(1_000);
    });
  }
  vi.useRealTimers();
}

describe("navigation intent on the rail's Contacts link", () => {
  it("warms the list's first read under the key the screen mounts, and the screen asks nothing more", async () => {
    const server = contactsServer();
    const client = signedIn();
    pointAt(rail(client), 500);

    await waitFor(() => expect(server.asked).toHaveLength(1));
    const url = new URL(server.asked[0]?.url ?? "");
    expect(url.searchParams.get("sort")).toBe("-created_at");
    expect(url.searchParams.get("limit")).toBe("50");
    server.answerAll();
    await waitFor(() =>
      expect(
        client.getQueryData(contactsArrivalOptions().queryKey)?.pages,
      ).toHaveLength(1),
    );

    cleanup();
    renderWith(client, <ContactsScreen />);
    expect(await screen.findByText("Anna Weber")).toBeTruthy();
    expect(server.asked).toHaveLength(1);
  });

  it("starts no read for a pointer that passes over the link", async () => {
    const server = contactsServer();
    const client = signedIn();
    pointAt(rail(client), 20, true);

    await act(() => Promise.resolve());
    expect(server.asked).toHaveLength(0);
    expect(
      client.getQueryState(contactsArrivalOptions().queryKey),
    ).toBeUndefined();
  });

  it("aborts a warm read the pointer left before it answered", async () => {
    const server = contactsServer();
    const client = signedIn();
    const link = rail(client);
    pointAt(link, 500);
    await waitFor(() => expect(server.asked).toHaveLength(1));

    fireEvent.pointerLeave(link, { pointerType: "mouse" });

    await waitFor(() => expect(server.asked[0]?.signal.aborted).toBe(true));
  });

  it("keeps a followed link's read alive after the pointer leaves, for the screen to adopt", async () => {
    const server = contactsServer();
    const client = signedIn();
    const link = rail(client);
    pointAt(link, 500);
    await waitFor(() => expect(server.asked).toHaveLength(1));

    fireEvent.click(link);
    fireEvent.pointerLeave(link, { pointerType: "mouse" });
    renderWith(client, <ContactsScreen />);
    server.answerAll();

    expect(await screen.findByText("Anna Weber")).toBeTruthy();
    expect(server.asked).toHaveLength(1);
    expect(server.asked[0]?.signal.aborted).toBe(false);
  });

  it("warms on keyboard focus too", async () => {
    const server = contactsServer();
    const client = signedIn();
    rail(client);
    const user = userEvent.setup();

    while (
      document.activeElement !== screen.getByRole("link", { name: "Contacts" })
    )
      await user.tab();

    await waitFor(() => expect(server.asked).toHaveLength(1));
  });

  it("starts nothing for a reader without contact read", async () => {
    const server = contactsServer();
    const client = signedIn(meFixture({ allow: { deal: ["read"] } }));
    navigationPrefetch(client).intend("contacts");

    await act(() => Promise.resolve());
    expect(server.asked).toHaveLength(0);
  });

  it("drops a warmed list when another principal signs in, so their screen reads its own", async () => {
    const server = contactsServer();
    const client = signedIn();
    navigationPrefetch(client).intend("contacts");
    await waitFor(() => expect(server.asked).toHaveLength(1));
    server.answerAll();
    await waitFor(() =>
      expect(
        client.getQueryData(contactsArrivalOptions().queryKey),
      ).toBeDefined(),
    );

    client.setQueryData(sessionOptions.queryKey, OTHER_READER);

    expect(
      client.getQueryState(contactsArrivalOptions().queryKey),
    ).toBeUndefined();
    renderWith(client, <ContactsScreen />);
    await waitFor(() => expect(server.asked).toHaveLength(2));
  });

  it("abandons a warm read still in flight when the principal changes", async () => {
    const server = contactsServer();
    const client = signedIn();
    navigationPrefetch(client).intend("contacts");
    await waitFor(() => expect(server.asked).toHaveLength(1));

    client.setQueryData(sessionOptions.queryKey, OTHER_READER);

    await waitFor(() => expect(server.asked[0]?.signal.aborted).toBe(true));
    server.answerAll();
    expect(
      client.getQueryState(contactsArrivalOptions().queryKey),
    ).toBeUndefined();
  });

  it("starts nothing while the session snapshot is being read again", async () => {
    const server = contactsServer();
    const client = signedIn();
    await client.invalidateQueries({
      queryKey: sessionOptions.queryKey,
      refetchType: "none",
    });

    navigationPrefetch(client).intend("contacts");

    await act(() => Promise.resolve());
    expect(server.asked).toHaveLength(0);
  });
});

describe("navigation intent toward Deals", () => {
  it("warms the default pipeline's first column, and the board joins it", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onDealsUrl: (url) => urls.push(url) }),
    );
    const client = signedIn();
    navigationPrefetch(client).intend("deals");

    await waitFor(() => expect(urls).toHaveLength(1));
    const column = new URL(urls[0] ?? "", "https://test.local");
    expect(column.searchParams.get("stage_id")).toBe("s1");
    expect(column.searchParams.get("pipeline_id")).toBe("pl");
    await waitFor(() =>
      expect(client.getQueryData(pipelinesOptions.queryKey)).toBeDefined(),
    );

    renderDeals(<DealsScreen />, client);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    const stagesRead = urls.map((url) =>
      new URL(url, "https://test.local").searchParams.get("stage_id"),
    );
    expect(stagesRead.filter((stage) => stage === "s1")).toHaveLength(1);
  });

  it("chooses the declared default pipeline and its earliest stage", () => {
    const pipeline = (id: string, isDefault: boolean): Pipeline => ({
      id,
      name: id,
      is_default: isDefault,
      position: 0,
      stages: [...stages]
        .reverse()
        .map((stage) => ({ ...stage, pipeline_id: id })),
    });

    const options = dealsArrivalColumnOptions([
      pipeline("first", false),
      pipeline("declared", true),
    ]);

    expect(options?.queryKey[2].pipelineId).toBe("declared");
    expect(options?.queryKey[4]).toBe("s1");
    expect(
      dealsArrivalColumnOptions([{ ...pipeline("bare", true), stages: [] }]),
    ).toBeUndefined();
  });
});
