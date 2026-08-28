/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LocaleProvider } from "../i18n";
import { RelationshipsTab } from "./relationships";

// A deal's stakeholders were readable on three surfaces — the rail's seats, the
// committee map, the coverage findings — and writable on none: the edge was
// creatable only from the PERSON's side, so seating a champion meant knowing
// which contact to open first. This is the generic relationships panel under a
// deal scope, which is what keeps create/edit/remove at one implementation.

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const PERSON = "01a02be9-2293-75d2-9dd2-3027d9b63dc2";

const stakeholder = {
  id: "rel-1",
  kind: "deal_stakeholder",
  deal_id: DEAL,
  person_id: PERSON,
  role: "champion",
  is_current_primary: false,
  source: "manual",
  captured_by: "human:u-1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Every request the panel makes. `seats` is what the deal_id list read answers,
// so the same stub serves the empty deal and the seated one.
function stubFetch(
  seats: unknown[],
  onPost: (body: unknown) => void = () => {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { method, url } = request;
      if (method === "POST") {
        onPost(JSON.parse(await request.text()));
        return json({ ...stakeholder, id: "rel-new" }, 201);
      }
      if (url.includes("/relationships")) {
        // The panel must ask by deal, not by person: a person_id read would
        // answer with a different deal's seats.
        expect(url).toContain(`deal_id=${DEAL}`);
        return json({ data: seats, page: { next_cursor: null } });
      }
      // The by-id read first: EntityRef resolves the far end that way, and the
      // list branch below would answer it with a page.
      if (url.includes(`/people/${PERSON}`)) {
        return json({ id: PERSON, full_name: "Mai Trần" });
      }
      if (url.includes("/people")) {
        return json({
          data: [{ id: PERSON, full_name: "Mai Trần" }],
          page: { next_cursor: null },
        });
      }
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
}

function renderPanel() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider>
        <RelationshipsTab scope={{ deal_id: DEAL }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The search debounces 250ms. Fake timers are armed BEFORE anything types, and
// userEvent is told to advance them, because switching them on afterwards leaves
// the already-scheduled timeout on the real clock: the test then waits out 250ms
// of wall time on a queue it shares with every other jsdom file, which is the
// flake family the frontend rulebook names rather than a wait at all.
function setup() {
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
}

function settleSearch() {
  act(() => {
    vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
  });
}

const SEARCH_DEBOUNCE_MS = 250;

beforeEach(() => {
  // shouldAdvanceTime, so the fake clock still ticks on its own: react-query's
  // own scheduling and testing-library's waiters run on timers too, and a clock
  // frozen until this file advances it hangs them rather than speeding them up.
  vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("the stakeholders on a deal", () => {
  it("says what it is, in the word the rest of the product uses", async () => {
    stubFetch([stakeholder]);
    renderPanel();

    expect(await screen.findByText("Stakeholders")).toBeTruthy();
    // "People & companies" is the person and company panel's heading, and it
    // sends a reader on a deal looking for a control this page does not have.
    expect(screen.queryByText("People & companies")).toBeNull();
  });

  // A deal anchors one kind, so a Kind column repeats one badge down every row
  // and a Kind picker asks a question with a single answer.
  it("drops the kind column and the kind picker on a single-kind scope", async () => {
    const user = setup();
    stubFetch([stakeholder]);
    renderPanel();

    await screen.findByTestId("remove-relationship");
    expect(screen.queryByText("Deal stakeholder")).toBeNull();
    await user.click(screen.getByTestId("add-relationship"));
    expect(
      within(screen.getByRole("dialog")).queryByLabelText("Kind"),
    ).toBeNull();
  });

  it("seats a person on THIS deal, naming both ends", async () => {
    const user = setup();
    const posted: unknown[] = [];
    stubFetch([], (body) => posted.push(body));
    renderPanel();

    await user.click(await screen.findByTestId("add-relationship"));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByLabelText("Role"), "economic_buyer");
    await user.type(dialog.getByPlaceholderText("Search…"), "mai");
    settleSearch();
    await user.click(await dialog.findByRole("button", { name: "Mai Trần" }));
    await user.click(screen.getByTestId("add-relationship-submit"));

    await waitFor(() => expect(posted.length).toBe(1));
    expect(posted[0]).toMatchObject({
      kind: "deal_stakeholder",
      deal_id: DEAL,
      person_id: PERSON,
      role: "economic_buyer",
      source: "manual",
    });
    // The deal is the anchor, so no organization travels: an endpoint shape the
    // rel_*_shape CHECKs refuse earns a 422 rather than a row.
    expect(posted[0]).not.toHaveProperty("organization_id");
  });

  it("says nobody is on the deal rather than nothing at all", async () => {
    stubFetch([]);
    renderPanel();

    expect(
      await screen.findByText("No stakeholder is recorded on this deal"),
    ).toBeTruthy();
  });

  // The committee map sits directly above this panel and the rail's seats beside
  // it; all three read GET /deals/{id}/coverage. Seating a champion filled this
  // table while the map one panel up still said the champion was missing.
  it("drops the coverage the map and the rail seats read", async () => {
    const user = setup();
    stubFetch([]);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const dropped: unknown[] = [];
    client.invalidateQueries = ((filters: { queryKey?: unknown }) => {
      dropped.push(filters?.queryKey);
      return Promise.resolve();
    }) as typeof client.invalidateQueries;
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <RelationshipsTab scope={{ deal_id: DEAL }} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    await user.click(await screen.findByTestId("add-relationship"));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByPlaceholderText("Search…"), "mai");
    settleSearch();
    await user.click(await dialog.findByRole("button", { name: "Mai Trần" }));
    await user.click(screen.getByTestId("add-relationship-submit"));

    await waitFor(() =>
      expect(dropped).toEqual(
        expect.arrayContaining([["relationships"], ["deal-coverage"]]),
      ),
    );
  });

  // The verbs are the whole point of adding this panel: the coverage view the
  // other three surfaces read carries no relationship id, so it can carry no
  // verb.
  it("offers edit and remove on a seat that is already there", async () => {
    stubFetch([stakeholder]);
    renderPanel();

    // Waited on the ROW, not the heading: the card draws its title before the
    // list read settles, so a heading is no evidence the seat arrived.
    expect(await screen.findByTestId("remove-relationship")).toBeTruthy();
    expect(screen.getByTestId("edit-record")).toBeTruthy();
  });
});
