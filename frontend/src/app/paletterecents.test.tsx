/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ENTITY_KINDS, type EntityKind } from "../lib/entity";
import { resetToSignedOut } from "../lib/session";
import {
  forgetSeat,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../lib/storage";
import { meFixture } from "../testing/mefixture";
import { steppedClock } from "../testing/steppedclock";
import { CommandPalette } from "./palette";
import {
  type RecentEntry,
  readRecents,
  writeRecents,
} from "./paletterecents.storage";

const me = meFixture({
  allow: {
    contact: ["read"],
    company: ["read"],
    lead: ["read"],
    deal: ["read"],
    project: ["read"],
  },
});
const id = "00000000-0000-4000-8000-000000000002";
const record: RecentEntry = {
  kind: "record",
  entity_type: "contact",
  id,
  label: "Stored private name",
};
const query: RecentEntry = { kind: "query", text: "Acme" };
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
afterEach(() => {
  cleanup();
  forgetSeat();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});
function mount(
  read: (request: Request) => Promise<Response> | Response = () =>
    json({ id, full_name: "Fresh name" }),
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], me);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      request.url.includes("/me")
        ? json(me)
        : request.url.includes("/search")
          ? json({ data: [], page: { has_more: false, next_cursor: null } })
          : read(request),
    ),
  );
  const close = vi.fn();
  const wrap = (open: boolean) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CommandPalette
          open={open}
          onClose={close}
          commands={[
            {
              id: "screen:deals",
              label: "Deals",
              type: "screen",
              route: { screen: "deals" },
            },
          ]}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const view = render(wrap(true));
  return {
    client,
    close,
    rerender: (open: boolean) => view.rerender(wrap(open)),
  };
}
it("keeps eight mixed entries and deduplicates queries without accents or case", () => {
  writeRecents(me.user.id, [
    record,
    { kind: "query", text: "CAFÉ" },
    { kind: "query", text: "cafe" },
    ...Array.from(
      { length: 10 },
      (_, n): RecentEntry => ({ kind: "query", text: `query ${n}` }),
    ),
  ]);
  expect(readRecents(me.user.id).entries).toHaveLength(8);
  expect(readRecents(me.user.id).entries.slice(0, 2)).toEqual([
    record,
    { kind: "query", text: "CAFÉ" },
  ]);
});
it("ignores another seat, unknown schemas, corrupt JSON and unsupported records", () => {
  writeRecents(me.user.id, [query]);
  expect(readRecents("other").entries).toEqual([]);
  for (const value of [
    "{",
    '{"version":2}',
    JSON.stringify({
      version: 1,
      seat: me.user.id,
      entries: [{ kind: "record", entity_type: "activity", id, label: "x" }],
    }),
  ]) {
    writeStored(STORAGE_KEYS.paletteRecents, value);
    expect(readRecents(me.user.id).entries).toEqual([]);
  }
});
it("never paints a stored name and shows the current name before the screens", async () => {
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record, query]);
  mount(() => pending.then((response) => response.clone()));
  expect(screen.queryByText(record.label)).toBeNull();
  expect(screen.getByText("Acme")).toBeTruthy();
  await act(async () => resolve(json({ id, full_name: "Renamed contact" })));
  expect(await screen.findByText("Renamed contact")).toBeTruthy();
  const rows = screen
    .getAllByRole("button")
    .filter((button) => button.classList.contains("palette-row"));
  expect(rows.map((row) => row.querySelector(".label")?.textContent)).toEqual([
    "Renamed contact",
    "Acme",
    "Deals",
  ]);
});
it.each([403, 404])("prunes a definitive %s refusal", async (status) => {
  writeRecents(me.user.id, [record, query]);
  mount(() =>
    json(
      { code: status === 403 ? "permission_denied" : "not_found", status },
      status,
    ),
  );
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([query]));
  expect(screen.queryByText(record.label)).toBeNull();
});
it("prunes archived records while keeping a transiently failed read for another opening", async () => {
  writeRecents(me.user.id, [record]);
  const view = mount(() => json({ code: "internal", status: 500 }, 500));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(readRecents(me.user.id).entries).toEqual([record]);
  view.rerender(false);
  cleanup();
  mount(() =>
    json({ id, full_name: "Archived", archived_at: "2026-01-01T00:00:00Z" }),
  );
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([]));
});
it("Clear cannot be undone by a late name response", async () => {
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record, query]);
  mount(() => pending.then((response) => response.clone()));
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  await act(async () => resolve(json({ id, full_name: "Too late" })));
  expect(readRecents(me.user.id).entries).toEqual([]);
  expect(screen.queryByText("Too late")).toBeNull();
});
it("a seat switch cannot paint or restore an earlier seat's late read", async () => {
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record]);
  const view = mount(() => pending.then((response) => response.clone()));
  act(() =>
    view.client.setQueryData(["me"], {
      ...me,
      user: { ...me.user, id: "00000000-0000-4000-8000-000000000003" },
    }),
  );
  await act(async () => resolve(json({ id, full_name: "Wrong seat" })));
  expect(screen.queryByText("Wrong seat")).toBeNull();
  expect(readRecents(me.user.id).entries).toEqual([record]);
});
it("replaying a query refills the box without closing or adding history", () => {
  writeRecents(me.user.id, [query]);
  const view = mount();
  fireEvent.keyDown(screen.getByRole("searchbox"), { key: "Enter" });
  expect(screen.getByRole("searchbox").getAttribute("value")).toBe("Acme");
  expect(view.close).not.toHaveBeenCalled();
  expect(readRecents(me.user.id).entries).toEqual([query]);
});
it("records an executed See all query but never typing or Ask", async () => {
  const view = mount();
  const input = screen.getByRole("searchbox");
  fireEvent.change(input, { target: { value: "Acme" } });
  expect(readRecents(me.user.id).entries).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([query]));
  expect(view.close).toHaveBeenCalledOnce();
});
it("rechecks access on activation and refuses a now-hidden record", async () => {
  let allowed = true;
  writeRecents(me.user.id, [record]);
  mount(() =>
    allowed
      ? json({ id, full_name: "Fresh name" })
      : json({ code: "not_found", status: 404 }, 404),
  );
  const row = await screen.findByRole("button", { name: /^Fresh name/ });
  allowed = false;
  fireEvent.click(row);
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([]));
  expect(window.location.hash).toBe("");
});
it("sign-out clears history and notifies the open palette", () => {
  writeRecents(me.user.id, [query]);
  mount();
  act(() => forgetSeat());
  expect(screen.queryByText("Acme")).toBeNull();
  removeStored(STORAGE_KEYS.paletteRecents);
});

it("rechecks fresh names on each opening", async () => {
  let name = "First name";
  writeRecents(me.user.id, [record]);
  const view = mount(() => json({ id, full_name: name }));
  await screen.findByText("First name");
  view.rerender(false);
  name = "Second name";
  view.rerender(true);
  expect(screen.queryByText("First name")).toBeNull();
  await screen.findByText("Second name");
});
it("hides a transient activation failure and keeps it for the next opening", async () => {
  let available = true;
  writeRecents(me.user.id, [record]);
  mount(() =>
    available
      ? json({ id, full_name: "Fresh name" })
      : json({ status: 503, code: "service_unavailable" }, 503),
  );
  const row = await screen.findByRole("button", { name: /^Fresh name/ });
  available = false;
  fireEvent.click(row);
  await waitFor(() => expect(screen.queryByText("Fresh name")).toBeNull());
  expect(readRecents(me.user.id).entries[0]).toMatchObject({ id });
  expect(window.location.hash).toBe("");
});
it("a closed palette never navigates when an activation finishes late", async () => {
  let waiting = false;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record]);
  const view = mount(() =>
    waiting ? pending : json({ id, full_name: "Fresh name" }),
  );
  const row = await screen.findByRole("button", { name: /^Fresh name/ });
  waiting = true;
  fireEvent.click(row);
  view.rerender(false);
  await act(async () => resolve(json({ id, full_name: "Late name" })));
  expect(window.location.hash).toBe("");
  expect(view.close).not.toHaveBeenCalled();
});
it("updates an open palette when another tab clears storage", () => {
  writeRecents(me.user.id, [query]);
  mount();
  act(() => {
    localStorage.removeItem(STORAGE_KEYS.paletteRecents.name);
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: STORAGE_KEYS.paletteRecents.name,
        newValue: null,
      }),
    );
  });
  expect(screen.queryByText("Acme")).toBeNull();
});

it.each(["contact", "company", "lead", "deal", "project"] as const)(
  "remembers an executed %s hit with its query",
  async (kind) => {
    const user = steppedClock();
    const view = mount();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) =>
        request.url.includes("/search")
          ? json({
              data: [{ type: kind, id, title: "Chosen record" }],
              page: { has_more: false, next_cursor: null },
            })
          : json(me),
      ),
    );
    await user.type(screen.getByRole("searchbox"), "Acme");
    await user.click(await screen.findByText("Chosen record"));
    expect(readRecents(me.user.id).entries).toEqual([
      { kind: "record", entity_type: kind, id, label: "Chosen record" },
      query,
    ]);
    expect(view.close).toHaveBeenCalledOnce();
  },
);

function metadata(kind: EntityKind, name: string, archived = false): Response {
  const archived_at = archived ? "2026-01-01T00:00:00Z" : null;
  if (kind === "company")
    return json({
      data: [
        {
          id,
          display_name: name,
          logo_url: "https://example.test/current-logo.png",
          archived_at,
        },
      ],
      page: { has_more: false, next_cursor: null },
    });
  if (kind === "contact" || kind === "lead")
    return json({ id, full_name: name, archived_at });
  return json({ id, name, archived_at });
}
it.each(ENTITY_KINDS)(
  "reads the authorized current name of a saved %s",
  async (kind) => {
    writeRecents(me.user.id, [
      { kind: "record", entity_type: kind, id, label: "Old private label" },
    ]);
    const read = vi.fn(() => metadata(kind, `Current ${kind}`));
    mount(read);
    const name = await screen.findByText(`Current ${kind}`);
    expect(screen.queryByText("Old private label")).toBeNull();
    expect(read).toHaveBeenCalledOnce();
    if (kind === "company")
      expect(
        name.closest("button")?.querySelector("img")?.getAttribute("src"),
      ).toBe("https://example.test/current-logo.png");
  },
);
it.each(ENTITY_KINDS)("removes an archived saved %s", async (kind) => {
  writeRecents(me.user.id, [
    { kind: "record", entity_type: kind, id, label: "Old" },
  ]);
  mount(() => metadata(kind, "Archived", true));
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([]));
  expect(screen.queryByText("Archived")).toBeNull();
});
it.each(ENTITY_KINDS)("removes an absent saved %s", async (kind) => {
  writeRecents(me.user.id, [
    { kind: "record", entity_type: kind, id, label: "Old" },
  ]);
  mount(() =>
    kind === "company"
      ? json({ data: [] })
      : json({ code: "not_found", status: 404 }, 404),
  );
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([]));
});
it("shows a renamed record when storage can be read but refuses writes", async () => {
  writeRecents(me.user.id, [record]);
  const read = vi.fn(() => json({ id, full_name: "Current name" }));
  const storage = localStorage;
  vi.stubGlobal("localStorage", {
    get length() {
      return storage.length;
    },
    getItem: storage.getItem.bind(storage),
    key: storage.key.bind(storage),
    removeItem: storage.removeItem.bind(storage),
    clear: storage.clear.bind(storage),
    setItem() {
      throw new DOMException("Full", "QuotaExceededError");
    },
  });
  mount(read);
  expect(await screen.findByText("Current name")).toBeTruthy();
  expect(read).toHaveBeenCalledOnce();
  expect(readRecents(me.user.id).entries).toEqual([record]);
});
it("keeps a keyboard-picked destination when checked records arrive above it", async () => {
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record, query]);
  const view = mount(() => pending.then((response) => response.clone()));
  const input = screen.getByRole("searchbox");
  fireEvent.keyDown(input, { key: "ArrowDown" });
  fireEvent.keyDown(input, { key: "ArrowDown" });
  expect(
    screen
      .getByRole("button", { name: "Deals Screen" })
      .classList.contains("selected"),
  ).toBe(true);
  await act(async () => resolve(json({ id, full_name: "Current name" })));
  expect(await screen.findByText("Current name")).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Deals Screen" })
      .classList.contains("selected"),
  ).toBe(true);
  fireEvent.keyDown(input, { key: "Enter" });
  expect(window.location.hash).toBe("#/deals");
  expect(view.close).toHaveBeenCalledOnce();
});
it("waits for a mixed recent list before default Enter opens its newest record", async () => {
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record, query]);
  const view = mount(() => pending.then((response) => response.clone()));
  const input = screen.getByRole("searchbox");
  fireEvent.keyDown(input, { key: "Enter" });
  expect(view.close).not.toHaveBeenCalled();
  expect(input.getAttribute("value")).toBe("");
  await act(async () => resolve(json({ id, full_name: "Current name" })));
  await screen.findByText("Current name");
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(window.location.hash).toBe(`#/contacts/${id}`));
});
it("returns focus to the query after clicking a saved search", async () => {
  const user = steppedClock();
  writeRecents(me.user.id, [query]);
  mount();
  await user.click(screen.getByRole("button", { name: "Acme" }));
  expect(document.activeElement).toBe(screen.getByRole("searchbox"));
});
it("keeps Clear reachable with only unnamed saved records and returns focus", async () => {
  const user = steppedClock();
  writeRecents(me.user.id, [record]);
  mount(() => json({ id, full_name: "" }));
  await waitFor(() => expect(screen.queryByText("Loading")).toBeNull());
  await user.click(screen.getByRole("button", { name: "Clear" }));
  expect(readRecents(me.user.id).entries).toEqual([]);
  expect(document.activeElement).toBe(screen.getByRole("searchbox"));
});
it("returns focus after an activation fails and hides its row", async () => {
  const user = steppedClock();
  let allowed = true;
  writeRecents(me.user.id, [record]);
  mount(() =>
    allowed
      ? json({ id, full_name: "Current name" })
      : json({ code: "service_unavailable", status: 503 }, 503),
  );
  const button = await screen.findByRole("button", { name: /Current name/ });
  allowed = false;
  await user.click(button);
  await waitFor(() => expect(screen.queryByText("Current name")).toBeNull());
  expect(document.activeElement).toBe(screen.getByRole("searchbox"));
});
it("hides a shown record before the changed permission read answers", async () => {
  let changed = false;
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record]);
  const view = mount(() => {
    calls++;
    return changed ? pending : json({ id, full_name: "Current name" });
  });
  await screen.findByText("Current name");
  changed = true;
  act(() =>
    view.client.setQueryData(
      ["me"],
      meFixture({ allow: { company: ["read"] } }),
    ),
  );
  await waitFor(() => expect(calls).toBe(2));
  expect(screen.queryByText("Current name")).toBeNull();
  await act(async () =>
    resolve(json({ code: "permission_denied", status: 403 }, 403)),
  );
  await waitFor(() => expect(readRecents(me.user.id).entries).toEqual([]));
});
it("a permission change aborts an in-flight activation", async () => {
  let changed = false;
  let waiting = false;
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record]);
  const view = mount(() => {
    calls++;
    return changed
      ? json({ code: "permission_denied", status: 403 }, 403)
      : waiting
        ? pending
        : json({ id, full_name: "Current name" });
  });
  const button = await screen.findByRole("button", { name: /Current name/ });
  waiting = true;
  fireEvent.click(button);
  await waitFor(() => expect(calls).toBe(2));
  changed = true;
  act(() =>
    view.client.setQueryData(
      ["me"],
      meFixture({ allow: { company: ["read"] } }),
    ),
  );
  await act(async () => resolve(json({ id, full_name: "Too late" })));
  expect(window.location.hash).toBe("");
  expect(view.close).not.toHaveBeenCalled();
  expect(screen.queryByText("Too late")).toBeNull();
});
it("the real sign-out reset erases history and the next seat sees none", async () => {
  writeRecents(me.user.id, [query]);
  const view = mount();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => json({ code: "unauthorized", status: 401 }, 401)),
  );
  await act(async () => {
    await resetToSignedOut(view.client);
  });
  expect(readRecents(me.user.id).entries).toEqual([]);
  expect(screen.queryByText("Acme")).toBeNull();
  act(() =>
    view.client.setQueryData(["me"], {
      ...me,
      user: { ...me.user, id: "00000000-0000-4000-8000-000000000003" },
    }),
  );
  expect(screen.queryByText("Acme")).toBeNull();
});
it("a late session proof cannot rewrite history cleared in another tab", async () => {
  writeRecents(me.user.id, [{ kind: "query", text: "Older search" }]);
  mount();
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  let probing = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.includes("/me")) {
        probing = true;
        return pending;
      }
      return json({ data: [], page: { has_more: false, next_cursor: null } });
    }),
  );
  fireEvent.change(screen.getByRole("searchbox"), {
    target: { value: "Acme" },
  });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(probing).toBe(true));
  act(() => {
    localStorage.removeItem(STORAGE_KEYS.paletteRecents.name);
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: STORAGE_KEYS.paletteRecents.name,
        newValue: null,
      }),
    );
  });
  await act(async () => resolve(json(me)));
  expect(readRecents(me.user.id).entries).toEqual([]);
});
it("a stale seat cannot remember an action after its server session ended", async () => {
  mount();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => json({ code: "unauthorized", status: 401 }, 401)),
  );
  fireEvent.change(screen.getByRole("searchbox"), {
    target: { value: "Acme" },
  });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  await act(async () => {});
  expect(readRecents(me.user.id).entries).toEqual([]);
});
it("two clicks share one activation and navigate only once", async () => {
  let waiting = false;
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record]);
  const view = mount(() => {
    calls++;
    return waiting ? pending : json({ id, full_name: "Current name" });
  });
  const button = await screen.findByRole("button", { name: /Current name/ });
  waiting = true;
  fireEvent.click(button);
  fireEvent.click(button);
  await waitFor(() => expect(calls).toBe(2));
  await act(async () => resolve(json({ id, full_name: "Current name" })));
  await waitFor(() => expect(view.close).toHaveBeenCalledOnce());
  fireEvent.click(button);
  expect(calls).toBe(2);
  expect(view.close).toHaveBeenCalledOnce();
});
it("keeps an activation when another tab appends an unrelated recent query", async () => {
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  writeRecents(me.user.id, [record, query]);
  const view = mount(() => {
    calls++;
    return calls === 2 ? pending : json({ id, full_name: "Current name" });
  });
  const button = await screen.findByRole("button", { name: /Current name/ });
  fireEvent.click(button);
  await waitFor(() => expect(calls).toBe(2));
  const raw = JSON.stringify({
    version: 1,
    seat: me.user.id,
    entries: [
      { kind: "query", text: "Another tab" },
      ...readRecents(me.user.id).entries,
    ],
  });
  act(() => {
    localStorage.setItem(STORAGE_KEYS.paletteRecents.name, raw);
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: STORAGE_KEYS.paletteRecents.name,
        newValue: raw,
      }),
    );
  });
  await act(async () => resolve(json({ id, full_name: "Current name" })));
  await waitFor(() => expect(view.close).toHaveBeenCalledOnce());
  expect(readRecents(me.user.id).entries).toContainEqual({
    kind: "query",
    text: "Another tab",
  });
});
it("invalid entries do not consume the eight valid slots and oversized values are refused", () => {
  writeStored(
    STORAGE_KEYS.paletteRecents,
    JSON.stringify({
      version: 1,
      seat: me.user.id,
      entries: [
        ...Array.from({ length: 8 }, () => ({
          kind: "record",
          entity_type: "activity",
          id,
          label: "bad",
        })),
        query,
      ],
    }),
  );
  expect(readRecents(me.user.id).entries).toEqual([query]);
  writeRecents(me.user.id, [{ kind: "query", text: "x".repeat(4097) }, query]);
  expect(readRecents(me.user.id).entries).toEqual([query]);
});
it("Ask does not remember its question", () => {
  mount();
  fireEvent.change(screen.getByRole("searchbox"), {
    target: { value: "Where are the documents?" },
  });
  fireEvent.click(screen.getByRole("button", { name: /Ask your documents/ }));
  expect(readRecents(me.user.id).entries).toEqual([]);
});

it("keeps executed searches in selection order when session reads finish later", async () => {
  mount();
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.includes("/me")) {
        calls++;
        return calls === 1 ? pending : json(me);
      }
      return json({ data: [], page: { has_more: false, next_cursor: null } });
    }),
  );
  const input = screen.getByRole("searchbox");
  fireEvent.change(input, { target: { value: "First search" } });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(calls).toBe(1));
  fireEvent.change(input, { target: { value: "Second search" } });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await act(async () => resolve(json(me)));
  await waitFor(() =>
    expect(readRecents(me.user.id).entries).toEqual([
      { kind: "query", text: "Second search" },
      { kind: "query", text: "First search" },
    ]),
  );
  expect(calls).toBe(2);
});

it("an empty-key sign-out cancels a pending write even after the same seat returns", async () => {
  const view = mount();
  let calls = 0;
  let resolve!: (response: Response) => void;
  const pending = new Promise<Response>((done) => {
    resolve = done;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.includes("/me")) {
        calls++;
        return calls === 1
          ? pending
          : json({ code: "unauthorized", status: 401 }, 401);
      }
      return json({ data: [], page: { has_more: false, next_cursor: null } });
    }),
  );
  fireEvent.change(screen.getByRole("searchbox"), {
    target: { value: "Acme" },
  });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(calls).toBe(1));
  await act(async () => {
    await resetToSignedOut(view.client);
  });
  act(() => view.client.setQueryData(["me"], me));
  await act(async () => resolve(json(me)));
  expect(readRecents(me.user.id).entries).toEqual([]);
});

it("Clear aborts a pending session check so another search can be remembered", async () => {
  writeRecents(me.user.id, [query]);
  mount();
  let calls = 0;
  let aborted = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (!request.url.includes("/me"))
        return json({ data: [], page: { has_more: false, next_cursor: null } });
      calls++;
      if (calls > 1) return json(me);
      return new Promise<Response>((_resolve, reject) => {
        request.signal.addEventListener(
          "abort",
          () => {
            aborted = true;
            reject(new DOMException("Aborted", "AbortError"));
          },
          { once: true },
        );
      });
    }),
  );
  const input = screen.getByRole("searchbox");
  fireEvent.change(input, { target: { value: "Old pending search" } });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() => expect(calls).toBe(1));
  fireEvent.change(input, { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  await waitFor(() => expect(aborted).toBe(true));
  fireEvent.change(input, { target: { value: "New search" } });
  fireEvent.click(screen.getByRole("button", { name: /See all results/ }));
  await waitFor(() =>
    expect(readRecents(me.user.id).entries).toEqual([
      { kind: "query", text: "New search" },
    ]),
  );
});

it("keeps Clear available when an archived row's prune cannot be saved", async () => {
  writeRecents(me.user.id, [record]);
  const storage = localStorage;
  vi.stubGlobal("localStorage", {
    get length() {
      return storage.length;
    },
    getItem: storage.getItem.bind(storage),
    key: storage.key.bind(storage),
    removeItem: storage.removeItem.bind(storage),
    clear: storage.clear.bind(storage),
    setItem() {
      throw new DOMException("Full", "QuotaExceededError");
    },
  });
  const read = vi.fn(() =>
    json({ id, full_name: "Archived", archived_at: "2026-01-01T00:00:00Z" }),
  );
  mount(read);
  await waitFor(() => expect(read).toHaveBeenCalledOnce());
  await act(async () => {});
  expect(screen.queryByText("Archived")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Clear" }));
  expect(readRecents(me.user.id).entries).toEqual([]);
});
