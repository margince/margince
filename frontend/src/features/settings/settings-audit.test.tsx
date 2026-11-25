/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../../components/listquery";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { auditHistoryHref } from "../../lib/auditloglink";
import { meFixture, READS_RECORDS } from "../../testing/mefixture";
import {
  auditEntry,
  jsonResponse,
  render,
} from "../../testing/settings.testkit";
import { steppedClock } from "../../testing/steppedclock";
import { AuditLogCard } from "./settings-audit";

// The fixture entry's toggle, named by its action and entity.
const EXPAND_UPDATE = "Show change detail: Updated on Contact";
const ANY_EXPAND = /^Show change detail: /;

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.useRealTimers();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

const VIEWER_ID = "00000000-0000-4000-8000-000000000001";
const RECORD_ID = "01a11ea4-37ab-720f-89a7-9008eb3f4280";

const created = {
  ...auditEntry,
  id: "al-created",
  actor_type: "human",
  actor_id: "human:u-anna",
  actor_name: "Anna Weber",
  passport_id: null,
  on_behalf_of: null,
  action: "create",
  entity_type: "onboarding_wizard_state",
  entity_id: RECORD_ID,
  entity_label: null,
  before: null,
  after: { step: "complete", settings: { voice: false } },
  authorization_rule:
    "role[individual] onboarding_wizard_state.create row_scope=own",
  evidence: null,
};

const ANNA = {
  id: "u-anna",
  email: "anna@example.test",
  display_name: "Anna Weber",
  status: "active",
  is_agent: false,
};

type Page = { entries: readonly object[]; next?: string | null };

function auditLogBackend(...pages: Page[]) {
  const answers = pages.length > 0 ? pages : [{ entries: [auditEntry] }];
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: ["admin"],
          allow: { ...READS_RECORDS, audit_log: ["read"] },
        }),
      );
    }
    if (url.includes("/v1/users")) {
      return jsonResponse({
        data: [ANNA],
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.includes("/audit-log")) {
      const page = url.includes("cursor=") ? answers[1] : answers[0];
      return jsonResponse({
        data: page.entries,
        page: { next_cursor: page.next ?? null, has_more: Boolean(page.next) },
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

function auditLogUrls(backend: ReturnType<typeof auditLogBackend>) {
  return backend.mock.calls
    .map(([input]) => String(input instanceof Request ? input.url : input))
    .filter((url) => url.includes("/audit-log"));
}

async function openDetail(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: ANY_EXPAND }));
}

describe("AuditLogCard", () => {
  // The page names the log, so its card carries no heading. The dials stand
  // open on one bar above it, each applying on its own.
  it("puts the filters on a bar above the log, and repeats no page heading", async () => {
    vi.stubGlobal("fetch", auditLogBackend());
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    const bar = screen.getByLabelText("Actor").closest("fieldset");
    expect(bar).not.toBeNull();
    expect(bar).not.toContainElement(screen.getByText("Updated"));
    expect(screen.getByRole("textbox", { name: "From" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "To" })).toBeInTheDocument();
    const calendars = screen.getAllByRole("button", { name: /^Choose date/ });
    expect(calendars).toHaveLength(2);
    for (const calendar of calendars) {
      expect(bar).toContainElement(calendar);
    }
    expect(screen.queryByRole("heading")).toBeNull();
    expect(document.querySelector("details")).toBeNull();
  });

  it("narrows the request to the filters, keeping the page size and dropping the cursor", async () => {
    const user = steppedClock();
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    expect(auditLogUrls(backend)[0]).toContain("limit=20");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Actor" }),
      "Anna Weber",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Entity type" }),
      "Contact",
    );
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

    await waitFor(() => {
      const latest = auditLogUrls(backend).at(-1) ?? "";
      expect(latest).toContain("actor=human%3Au-anna");
      expect(latest).toContain("entity_type=contact");
    });
    expect(
      screen.getByText("Includes what an AI assistant did on their behalf."),
    ).toBeInTheDocument();
    const latest = auditLogUrls(backend).at(-1) ?? "";
    expect(latest).toContain("limit=20");
    expect(latest).not.toContain("cursor=");
  });

  it("opens on the record a link names, with both dials filled", async () => {
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    window.location.hash = auditHistoryHref("approval", RECORD_ID);
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    expect(
      screen.getByRole("combobox", { name: "Entity type" }),
    ).toHaveTextContent(/approval/i);
    const first = auditLogUrls(backend)[0] ?? "";
    expect(first).toContain("entity_type=approval");
    expect(first).toContain(`entity_id=${RECORD_ID}`);
  });

  it("follows a second link while the log is open", async () => {
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    window.location.hash = auditHistoryHref("approval", RECORD_ID);
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    const second = "01890000-0000-7000-8000-000000000002";
    window.location.hash = auditHistoryHref("approval", second);
    window.dispatchEvent(new HashChangeEvent("hashchange"));
    await waitFor(() => {
      expect(auditLogUrls(backend).at(-1)).toContain(`entity_id=${second}`);
    });
  });

  it("holds a part of a record ID at its field and asks the server only for a whole one", async () => {
    const user = userEvent.setup();
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    const field = screen.getByRole("textbox", { name: "Entity ID" });
    await user.type(field, RECORD_ID.slice(-8));
    expect(
      screen.getByText(
        "Paste the full record ID. Use Copy record ID on a row.",
      ),
    ).toBeInTheDocument();
    expect(field).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Updated")).toBeInTheDocument();
    expect(
      auditLogUrls(backend).some((url) => url.includes("entity_id=")),
    ).toBe(false);

    await user.clear(field);
    await user.paste(RECORD_ID);
    await waitFor(() => {
      expect(auditLogUrls(backend).at(-1)).toContain(`entity_id=${RECORD_ID}`);
    });
    expect(field).not.toHaveAttribute("aria-invalid");
  });

  it("says a reversed window at its field and asks the server nothing for it", async () => {
    const user = userEvent.setup();
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    await user.type(
      screen.getByRole("textbox", { name: "From" }),
      "2026-11-17{Enter}",
    );
    await user.type(
      screen.getByRole("textbox", { name: "To" }),
      "2026-11-01{Enter}",
    );

    await waitFor(() => {
      expect(
        screen.getAllByText("Pick a To date on or after the From date."),
      ).toHaveLength(2);
    });
    expect(
      auditLogUrls(backend).some(
        (url) => url.includes("from=") && url.includes("to="),
      ),
    ).toBe(false);
  });

  it("keeps every filter in the address, so a reload opens on the same view", async () => {
    const user = userEvent.setup();
    const backend = auditLogBackend();
    vi.stubGlobal("fetch", backend);
    window.location.hash = "#/settings/audit?actor=human%3Au-anna&action=erase";
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    const first = auditLogUrls(backend)[0] ?? "";
    expect(first).toContain("actor=human%3Au-anna");
    expect(first).toContain("action=erase");
    expect(
      await screen.findByRole("combobox", { name: "Actor" }),
    ).toHaveTextContent("Anna Weber");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Entity type" }),
      "Contact",
    );
    await waitFor(() => {
      expect(window.location.hash).toContain("entity_type=contact");
    });
    expect(window.location.hash).toContain("action=erase");
  });

  it("says the log is empty before any filter, and that nothing matches after one", async () => {
    const user = steppedClock();
    vi.stubGlobal("fetch", auditLogBackend({ entries: [] }));
    render(<AuditLogCard />);
    expect(await screen.findByText("Nothing here yet.")).toBeInTheDocument();

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Action" }),
      "Erased",
    );
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
    expect(
      await screen.findByText("No recorded actions match these filters."),
    ).toBeInTheDocument();
    // The dial shows the label and asks the server with the contract's token.
    const asked = vi
      .mocked(fetch)
      .mock.calls.map(([input]) =>
        String(input instanceof Request ? input.url : input),
      );
    expect(asked.some((url) => url.includes("action=erase"))).toBe(true);
  });

  it("offers a retry when the log fails to load, and keeps the filters", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({
              roles: ["admin"],
              allow: { ...READS_RECORDS, audit_log: ["read"] },
            }),
          );
        }
        return jsonResponse({ title: "Upstream is down" }, 500);
      }),
    );
    render(<AuditLogCard />);
    expect(
      await screen.findByRole("button", { name: "Retry" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Could not load this view. Reload the page."),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Actor")).toBeInTheDocument();
  });

  it("names the row its toggle opens, and points at a region in the document", async () => {
    vi.stubGlobal("fetch", auditLogBackend());
    render(<AuditLogCard />);
    const user = userEvent.setup();
    const toggle = await screen.findByRole("button", { name: EXPAND_UPDATE });

    for (const expanded of ["false", "true"]) {
      expect(toggle).toHaveAttribute("aria-expanded", expanded);
      const controls = toggle.getAttribute("aria-controls") ?? "";
      expect(document.getElementById(controls)).not.toBeNull();
      await user.click(toggle);
    }
  });

  it("names the toggle in the same words the row shows, never the raw key", async () => {
    vi.stubGlobal("fetch", auditLogBackend({ entries: [created] }));
    render(<AuditLogCard />);
    expect(
      await screen.findByRole("button", {
        name: `Show change detail: Created on ${en["audit.kind.onboarding_wizard_state"]}`,
      }),
    ).toBeInTheDocument();
  });

  it("reads each entry under the column it belongs to", async () => {
    vi.stubGlobal("fetch", auditLogBackend({ entries: [created] }));
    render(<AuditLogCard />);
    const table = await screen.findByRole("table", {
      name: "Recorded actions",
    });
    const headers = within(table)
      .getAllByRole("columnheader")
      .map((header) => header.textContent);
    expect(headers).toEqual(["When", "Actor", "Action", "Target", "Detail"]);
    const [, row] = within(table).getAllByRole("row");
    const cells = within(row).getAllByRole("cell");
    expect(cells[1]).toHaveTextContent("Anna Weber");
    expect(cells[2]).toHaveTextContent("Created");
    expect(cells[3]).toHaveTextContent(
      en["audit.kind.onboarding_wizard_state"],
    );
  });

  it.each([
    ["create", "Created", "badge-success"],
    ["delete", "Deleted", "badge-danger"],
  ])(
    "tones a %s as %s, %s, and every other verb neutral",
    async (action, label, tone) => {
      vi.stubGlobal(
        "fetch",
        auditLogBackend({ entries: [{ ...created, action }, auditEntry] }),
      );
      render(<AuditLogCard />);
      expect((await screen.findByText(label)).closest(".badge")).toHaveClass(
        tone,
      );
      expect(screen.getByText("Updated").closest(".badge")).toHaveAttribute(
        "class",
        "badge",
      );
    },
  );

  it("states when an entry happened against the clock, with the instant on hover", async () => {
    steppedClock();
    vi.setSystemTime(new Date("2026-07-10T12:00:00Z"));
    vi.stubGlobal("fetch", auditLogBackend());
    render(<AuditLogCard />);
    const when = await screen.findByText("3 hours ago");
    expect(when).toHaveAttribute("datetime", auditEntry.occurred_at);
    expect(when.getAttribute("title")).toMatch(/2026/);
  });

  it("names the record from its label and links a record that has a page", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend({
        entries: [{ ...auditEntry, entity_label: "Priya Shah" }],
      }),
    );
    render(<AuditLogCard />);
    const link = await screen.findByRole("link", { name: "Priya Shah" });
    expect(link).toHaveAttribute("href", "/#/contacts/p-1");
  });

  it("names a record without a label by its kind and its id's random tail, and reads no record to name it", async () => {
    const backend = auditLogBackend({ entries: [created] });
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    const shortId = await screen.findByText("eb3f4280");
    expect(shortId.tagName).toBe("CODE");
    expect(shortId).toHaveAttribute("title", RECORD_ID);
    expect(screen.queryByText(RECORD_ID.slice(0, 8))).toBeNull();
    expect(
      screen.getByRole("button", { name: "Copy record ID" }),
    ).toBeInTheDocument();
    const asked = backend.mock.calls.map(
      ([input]) =>
        new URL(
          String(input instanceof Request ? input.url : input),
          "http://localhost",
        ).pathname,
    );
    // The names read is the actor's picture and the roster fills the Actor dial;
    // no record is read to name the row.
    const allowed = new Set([
      "/v1/me",
      "/v1/audit-log",
      "/v1/users",
      "/v1/users/names",
    ]);
    expect(asked.filter((path) => !allowed.has(path))).toEqual([]);
  });

  it("keeps the change detail hidden until the row is expanded", async () => {
    vi.stubGlobal("fetch", auditLogBackend());
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    const toggle = screen.getByRole("button", { name: EXPAND_UPDATE });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("qualified")).toBeNull();

    await openDetail(user);

    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const detail = document.getElementById(
      toggle.getAttribute("aria-controls") ?? "",
    );
    expect(detail).toHaveTextContent("qualified");
    expect(detail).toHaveTextContent("An agent");
    expect(detail).not.toHaveTextContent("pp-9");
  });

  it("names the client the row's own passport recorded, though the reader's passports no longer list it", async () => {
    const rotated = { ...auditEntry, agent_client: "Claude Desktop" };
    vi.stubGlobal("fetch", auditLogBackend({ entries: [rotated] }));
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    await openDetail(user);

    const toggle = screen.getByRole("button", { name: EXPAND_UPDATE });
    const detail = document.getElementById(
      toggle.getAttribute("aria-controls") ?? "",
    );
    expect(detail).toHaveTextContent("Claude Desktop");
    expect(detail).not.toHaveTextContent("An agent");
  });

  it("opens an entry from anywhere on its row, its id included, but not from a control in it", async () => {
    vi.stubGlobal("fetch", auditLogBackend({ entries: [created] }));
    const user = userEvent.setup();
    render(<AuditLogCard />);
    const toggle = await screen.findByRole("button", { name: ANY_EXPAND });

    await user.click(screen.getByText("eb3f4280"));
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    await user.click(screen.getByRole("button", { name: "Copy record ID" }));
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    await user.click(screen.getByText("Anna Weber"));
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("shows a created record's values alone, and a structured one as code", async () => {
    vi.stubGlobal("fetch", auditLogBackend({ entries: [created] }));
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Anna Weber");
    await openDetail(user);

    const fields = screen.getByText("step").closest("dl");
    if (!fields) {
      throw new Error("the change detail drew no field list");
    }
    expect(within(fields).getByText("complete")).toBeInTheDocument();
    expect(within(fields).getByText('{"voice":false}').tagName).toBe("CODE");
    expect(within(fields).queryByText("(created)")).toBeNull();
    expect(screen.queryByRole("table", { name: /changed fields/i })).toBeNull();
  });

  it("strikes the value an update replaced and keeps the one it wrote", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend({
        entries: [
          {
            ...auditEntry,
            before: { address: { city: "Berlin" } },
            after: { address: { city: "Munich" } },
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    await openDetail(user);

    expect(screen.getByText("address").tagName).toBe("DT");
    expect(screen.getByText('{"city":"Berlin"}')).toHaveClass(
      "field-diff-from",
    );
    expect(screen.getByText('{"city":"Munich"}')).toHaveClass("field-diff-to");
    expect(screen.queryByText("[object Object]")).toBeNull();
  });

  it("shows what a removal took away", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend({
        entries: [
          {
            ...created,
            action: "delete",
            before: { name: "Old list", next_step: null },
            after: null,
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Deleted");
    await openDetail(user);
    expect(screen.getByText("Old list")).toHaveClass("auditlog-value");
    // A column the record held empty reads as no value, not as a creation.
    expect(screen.getByText("No value")).toBeInTheDocument();
    expect(screen.queryByText("(cleared)")).toBeNull();
    expect(screen.queryByText("(created)")).toBeNull();
  });

  it.each([
    [
      "a role policy in words",
      "role[admin] activity.create row_scope=all",
      "Admin role · activity create · all records",
    ],
    [
      "several roles under one plural",
      "role[rep,individual] deal.update row_scope=team",
      "User, individual roles · deal update · team records",
    ],
    ["a system write", "system", "System, no role check"],
    ["a Deal Room write", "deal_room_session", "Deal Room session"],
    ["an unknown shape as written", "role:admin", "role:admin"],
  ])("says what allowed the write: %s", async (_case, rule, words) => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend({
        entries: [{ ...auditEntry, authorization_rule: rule }],
      }),
    );
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    await openDetail(user);
    const term = screen.getByText("Allowed by");
    expect(term.nextElementSibling).toHaveTextContent(words);
  });

  it.each([
    ["the resolved name", { on_behalf_of_name: "Anna Weber" }, "Anna Weber"],
    ["a stand-in when no name resolved", {}, "Unknown member"],
    ["the viewer as You", { on_behalf_of: VIEWER_ID }, "You"],
  ])(
    "names an agent's human authority as %s, with the agent under it",
    async (_case, fields, expected) => {
      vi.stubGlobal(
        "fetch",
        auditLogBackend({ entries: [{ ...auditEntry, ...fields }] }),
      );
      render(<AuditLogCard />);
      await screen.findByText("Updated");
      const name = await screen.findByText(expected);
      expect(name.closest(".auditlog-who")).toHaveTextContent("via an agent");
      expect(name.closest("td")).not.toHaveTextContent(VIEWER_ID);
    },
  );

  it("says a target with no id has no value, and offers nothing to copy", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend({ entries: [{ ...auditEntry, entity_id: undefined }] }),
    );
    render(<AuditLogCard />);

    const none = await screen.findByText(en["settings.auditNoValue"]);
    const cell = none.closest("td");
    if (!(cell instanceof HTMLElement)) throw new Error("no target cell");
    expect(
      within(cell).queryByRole("button", { name: en["settings.auditCopyId"] }),
    ).toBeNull();
  });

  it("loads the next page under the first", async () => {
    const backend = auditLogBackend(
      { entries: [auditEntry], next: "c-2" },
      { entries: [{ ...created, id: "al-next" }] },
    );
    vi.stubGlobal("fetch", backend);
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    await user.click(screen.getByRole("button", { name: "Load more" }));

    expect(await screen.findByText("Anna Weber")).toBeInTheDocument();
    expect(screen.getByText("Updated")).toBeInTheDocument();
    expect(auditLogUrls(backend).at(-1)).toContain("cursor=c-2");
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });
});
