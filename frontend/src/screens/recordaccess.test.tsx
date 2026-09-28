/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import { useContact360 } from "./contact360";
import { RecordAccess } from "./recordaccess";

type Contact = components["schemas"]["Contact"];
type Company = components["schemas"]["Company"];
type Kind = "contact" | "company";

const base: Contact = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  // The write pins the row it overwrites, so a fixture with no version is a
  // row this panel refuses to write — see "refuses to write a row…" below.
  version: 7,
};

type Seat = Readonly<{
  seat_type: "full" | "read";
  objects: Record<string, { read: boolean; update: boolean }>;
}>;

const mayWrite: Seat = {
  seat_type: "full",
  objects: {
    contact: { read: true, update: true },
    company: { read: true, update: true },
  },
};

const ROSTER = {
  data: [{ id: "u-owner", display_name: "Mira Voss" }],
  page: { next_cursor: null, has_more: false },
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const me = (seat: Seat) =>
  json({
    user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
    authorization: seat,
  });

function stub(
  options: Readonly<{
    sent?: string[];
    writes?: { body: unknown; version: string | null }[];
    status?: number;
    seat?: Seat;
    roster?: unknown;
  }> = {},
) {
  const { sent = [], writes = [], status = 200, seat = mayWrite } = options;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push(key);
      if (key === "GET /me") return me(seat);
      if (key === "GET /users") return json(options.roster ?? ROSTER);
      if (method === "PATCH" && request) {
        writes.push({
          body: await request.json(),
          version: request.headers.get("If-Match"),
        });
        if (status !== 200) return json({ status, title: "Conflict" }, status);
      }
      return json({});
    }),
  );
}

function LiveRecordAccess() {
  const view = useContact360(base.id);
  return view.data ? (
    <RecordAccess kind="contact" record={view.data.contact} />
  ) : null;
}

function draw(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        {node}
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
}

const drawContact = (contact?: Contact) =>
  draw(
    contact ? (
      <RecordAccess kind="contact" record={contact} />
    ) : (
      <LiveRecordAccess />
    ),
  );

// The chip is named by the question it answers; its visible word is the state.
const chip = (kind: Kind = "contact") =>
  screen.findByRole("button", {
    name: new RegExp(en[`recordAccess.${kind}.title`]),
  });

async function open(user: ReturnType<typeof userEvent.setup>, kind?: Kind) {
  await user.click(await chip(kind));
  return screen.findByRole("region", {
    name: new RegExp(en[`recordAccess.${kind ?? "contact"}.title`]),
  });
}

const radio = (panel: HTMLElement, key: "owner" | "workspace") =>
  within(panel).getByRole("radio", {
    name: new RegExp(`^${en[`recordAccess.option.${key}`]}`),
  });

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
});

describe("RecordAccess — the chip", () => {
  it("says a shared contact is the company's, and opens nothing until pressed", async () => {
    stub();
    drawContact({ ...base, visibility: "workspace", writable: true });
    const control = await chip();
    // "Shared" and not "Team": the column has no team value, and the answer
    // behind this chip says everyone in the company can see the record.
    expect(control.textContent).toContain("Shared");
    expect(control.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.queryByText(en["recordAccess.manage"])).toBeNull();
  });

  it("names a private record's state, never the reader, on the chip", async () => {
    stub();
    drawContact({
      ...base,
      visibility: "owner",
      writable: false,
      owner_id: "u-owner",
    });
    const control = await chip();
    expect(control.textContent).toContain(en["visibility.private"]);
    expect(control.textContent).not.toMatch(/only you/i);
  });

  it("opens into the panel, and Escape closes it back onto the chip", async () => {
    const user = userEvent.setup();
    stub();
    drawContact({ ...base, visibility: "workspace", writable: true });
    const panel = await open(user);
    expect((await chip()).getAttribute("aria-expanded")).toBe("true");
    await waitFor(() =>
      expect(panel.contains(document.activeElement)).toBe(true),
    );
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("region")).toBeNull();
    expect(document.activeElement).toBe(await chip());
  });

  it("takes the reader to the share screen for the full answer", async () => {
    const user = userEvent.setup();
    stub();
    drawContact({ ...base, visibility: "workspace", writable: false });
    const panel = await open(user);
    await user.click(
      within(panel).getByRole("button", { name: en["recordAccess.manage"] }),
    );
    expect(window.location.hash).toBe("#/share/contact/p-1");
  });

  it("draws nothing at all when the server sent no visibility", () => {
    // A server too old to send it, or a path that does not. Guessing
    // `workspace` would tell a reader their private contact is public.
    stub();
    drawContact({ ...base, writable: true });
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("RecordAccess — who the sentence names", () => {
  it("tells the owner the contact is theirs and their sharers'", async () => {
    const user = userEvent.setup();
    stub();
    drawContact({
      ...base,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    expect(
      await within(panel).findByText(en["recordAccess.contact.privateYours"]),
    ).toBeTruthy();
  });

  it("names the owner to a colleague the private contact was shared with", async () => {
    const user = userEvent.setup();
    stub();
    drawContact({
      ...base,
      visibility: "owner",
      writable: false,
      owner_id: "u-owner",
    });
    const panel = await open(user);
    // An administrator's role does not lift owner-privacy, so the one reason a
    // colleague is reading it is a share — and "Only you" would be false.
    expect(
      await within(panel).findByText(/^Private to Mira Voss\. You can see/),
    ).toBeTruthy();
  });

  it("says 'its owner' when the roster cannot name them", async () => {
    const user = userEvent.setup();
    stub({
      roster: { data: [], page: { next_cursor: null, has_more: false } },
    });
    drawContact({
      ...base,
      visibility: "owner",
      writable: false,
      owner_id: "u-owner",
    });
    const panel = await open(user);
    expect(
      await within(panel).findByText(en["recordAccess.contact.privateOfOwner"]),
    ).toBeTruthy();
  });
});

describe("RecordAccess — the switch", () => {
  it("publishes a private contact through the ordinary contact patch", async () => {
    const user = userEvent.setup();
    const sent: string[] = [];
    const writes: { body: unknown; version: string | null }[] = [];
    stub({ sent, writes });
    drawContact({
      ...base,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    await user.click(radio(panel, "workspace"));
    expect(sent).toContain("PATCH /contacts/p-1");
    expect(writes).toEqual([
      { body: { visibility: "workspace" }, version: "7" },
    ]);
    expect(
      await screen.findByText(en["recordAccess.contact.published"]),
    ).toBeTruthy();
  });

  it("makes a workspace contact private again", async () => {
    const user = userEvent.setup();
    const writes: { body: unknown; version: string | null }[] = [];
    stub({ writes });
    drawContact({
      ...base,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    await user.click(radio(panel, "owner"));
    expect(writes).toEqual([{ body: { visibility: "owner" }, version: "7" }]);
  });

  it("offers the switch to a colleague holding a write grant", async () => {
    const user = userEvent.setup();
    // The patch path runs the ordinary write test (object grant plus
    // EnsureWritable), so a grant holder the server would admit is offered it.
    const sent: string[] = [];
    stub({ sent });
    drawContact({
      ...base,
      visibility: "owner",
      writable: true,
      owner_id: "u-owner",
    });
    const panel = await open(user);
    await user.click(radio(panel, "workspace"));
    expect(sent).toContain("PATCH /contacts/p-1");
  });

  it("writes nothing when the current answer is picked again", async () => {
    const user = userEvent.setup();
    const sent: string[] = [];
    stub({ sent });
    drawContact({
      ...base,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    await user.click(radio(panel, "workspace"));
    expect(sent).not.toContain("PATCH /contacts/p-1");
  });

  it("refuses to write a row it read back without a version", async () => {
    const user = userEvent.setup();
    // Unpinned is last-write-wins, and this write moves the column that
    // decides who may read the record. The refusal surfaces through the
    // mutation's error path rather than sending an unconditional PATCH.
    const sent: string[] = [];
    stub({ sent });
    drawContact({
      ...base,
      version: undefined,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    await user.click(radio(panel, "owner"));
    expect(await within(panel).findByRole("alert")).toBeTruthy();
    expect(sent).not.toContain("PATCH /contacts/p-1");
  });

  it("keeps the private mark and explains a refused write inside the panel", async () => {
    const user = userEvent.setup();
    stub({ status: 409 });
    drawContact({
      ...base,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user);
    await user.click(radio(panel, "workspace"));
    expect(await within(panel).findByRole("alert")).toBeTruthy();
    expect((await chip()).textContent).toContain(en["visibility.private"]);
    // The switch falls back to the stored answer once the write is refused.
    expect(radio(panel, "owner")).toHaveProperty("checked", true);
    expect(screen.queryByText(en["recordAccess.contact.published"])).toBeNull();
  });

  it("refreshes a stale version, then shares and makes private again", async () => {
    const user = userEvent.setup();
    let version = 7;
    let visibility: "owner" | "workspace" = "owner";
    const writes: { version: string | null; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (!(input instanceof Request))
          throw new Error("Expected an API request");
        if (new URL(input.url).pathname.endsWith("/me")) return me(mayWrite);
        if (input.method === "PATCH") {
          const body: unknown = await input.json();
          writes.push({ version: input.headers.get("If-Match"), body });
          version++;
          if (writes.length === 1)
            return json({ status: 409, code: "version_skew" }, 409);
          if (
            typeof body === "object" &&
            body !== null &&
            "visibility" in body &&
            (body.visibility === "owner" || body.visibility === "workspace")
          ) {
            visibility = body.visibility;
          }
          return json({});
        }
        return json({
          contact: {
            ...base,
            version,
            visibility,
            writable: true,
            owner_id: "u1",
          },
        });
      }),
    );
    drawContact();
    const panel = await open(user);
    await user.click(radio(panel, "workspace"));
    expect((await within(panel).findByRole("alert")).textContent).toBe(
      en["edit.versionSkew"],
    );
    await user.click(radio(panel, "workspace"));
    await within(panel).findByText(en["recordAccess.contact.shared"]);
    await user.click(radio(panel, "owner"));
    await within(panel).findByText(en["recordAccess.contact.privateYours"]);
    expect(writes).toEqual([
      { version: "7", body: { visibility: "workspace" } },
      { version: "8", body: { visibility: "workspace" } },
      { version: "9", body: { visibility: "owner" } },
    ]);
  });
});

describe("RecordAccess — a reader who may not change it", () => {
  it.each<[string, Partial<Contact>, Seat, string]>([
    [
      "a row the server says is not theirs",
      { writable: false, owner_id: "u-owner" },
      mayWrite,
      en["contact.notYoursToChange"],
    ],
    [
      "a role without the contact update grant",
      { writable: true, owner_id: "u1" },
      {
        seat_type: "full",
        objects: { contact: { read: true, update: false } },
      },
      en["contact.notYoursToChange"],
    ],
    [
      "a read seat",
      { writable: true, owner_id: "u1" },
      { ...mayWrite, seat_type: "read" },
      en["contact.notYoursToChange"],
    ],
    [
      "an archived contact",
      { writable: true, owner_id: "u1", archived_at: "2026-08-02T00:00:00Z" },
      mayWrite,
      en["contact.rail.archivedReadOnly"],
    ],
  ])(
    "says why instead of offering the switch: %s",
    async (_, row, seat, why) => {
      const user = userEvent.setup();
      stub({ seat });
      drawContact({ ...base, visibility: "workspace", ...row });
      const panel = await open(user);
      // The page's own sentence for its other writes, so the header cannot give
      // a reason the page beside it does not.
      expect(await within(panel).findByText(why)).toBeTruthy();
      expect(within(panel).queryByRole("radio")).toBeNull();
      // The way to the full answer stays; only the switch goes.
      expect(
        within(panel).getByRole("button", { name: en["recordAccess.manage"] }),
      ).toBeTruthy();
    },
  );
});

// The company half. The same component, so the cases that are about the
// COMPONENT are not repeated here — these are the ones that would pass on a
// contact and still ship a broken company: the endpoint it writes to, the
// company noun, and the company page's own refusal.
describe("RecordAccess — a company", () => {
  const company: Company = {
    id: "c-1",
    display_name: "Weber GmbH",
    source: "gmail:seed",
    captured_by: "connector:gmail",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    version: 4,
  };

  const drawCompany = (record: Company) =>
    draw(<RecordAccess kind="company" record={record} />);

  it("publishes a private company through the company patch", async () => {
    const user = userEvent.setup();
    // A company capture minted owner-scoped could only be widened by a sender
    // verdict before this, so one the classifier never judged stayed private
    // with nothing a human could press.
    const sent: string[] = [];
    const writes: { body: unknown; version: string | null }[] = [];
    stub({ sent, writes });
    drawCompany({
      ...company,
      visibility: "owner",
      writable: true,
      owner_id: "u1",
    });
    const panel = await open(user, "company");
    expect(
      await within(panel).findByText(en["recordAccess.company.privateYours"]),
    ).toBeTruthy();
    await user.click(radio(panel, "workspace"));
    // The COMPANY endpoint: a component that wrote /contacts/c-1 would pass
    // every other assertion in this block.
    expect(sent).toContain("PATCH /companies/c-1");
    expect(writes).toEqual([
      { body: { visibility: "workspace" }, version: "4" },
    ]);
  });

  it("names the company rather than the contact on its chip", async () => {
    stub();
    drawCompany({
      ...company,
      visibility: "workspace",
      writable: true,
      owner_id: "u1",
    });
    // One string shared with the contact header would call a company a
    // contact in the first words a screen reader hears.
    expect(await chip("company")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: new RegExp(en["recordAccess.contact.title"]),
      }),
    ).toBeNull();
  });

  it("gives the company page's own refusal on a company not theirs", async () => {
    const user = userEvent.setup();
    stub();
    drawCompany({
      ...company,
      visibility: "owner",
      writable: false,
      owner_id: "u-owner",
    });
    const panel = await open(user, "company");
    expect(
      await within(panel).findByText(en["record.notYoursToChange"]),
    ).toBeTruthy();
    expect(within(panel).queryByRole("radio")).toBeNull();
  });

  it("draws nothing at all when the server sent no visibility", () => {
    stub();
    drawCompany({ ...company, writable: true });
    expect(screen.queryByRole("button")).toBeNull();
  });
});
