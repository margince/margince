/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { steppedClock } from "../testing/steppedclock";
import { SEARCH_DEBOUNCE_MS } from "./listquery";
import { AuditLogCard } from "./settings.auditlog";
import { auditWhenRange } from "./settings.auditlog.vocab";
import { auditEntry, jsonResponse, render } from "./settings.testkit";

// The audit trail card, read on its own rather than through the page that
// hosts it: a toolbar always in view, a table of changes that names records in
// words, the wire as the only honest witness that a filter narrowed the
// question, and a change detail that stays out of the table until asked for.

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

type AuditAnswer = Readonly<{ status?: number; body: unknown }>;

// A principal holding `audit_log:read`, which `GET /v1/audit-log` asks for,
// and an audit log answering whatever `answer` says for the URL it was asked.
function auditLogBackend(answer: (url: string) => AuditAnswer) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({ roles: ["admin"], allow: { audit_log: ["read"] } }),
      );
    }
    if (url.includes("/audit-log")) {
      const { status, body } = answer(url);
      return jsonResponse(body, status);
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

function page(entries: readonly unknown[]): AuditAnswer {
  return {
    body: { data: entries, page: { next_cursor: null, has_more: false } },
  };
}

// Which /audit-log URLs a backend was actually asked for, newest last.
function auditLogUrls(backend: ReturnType<typeof auditLogBackend>) {
  return backend.mock.calls
    .map(([input]) => String(input instanceof Request ? input.url : input))
    .filter((url) => url.includes("/audit-log"));
}

describe("AuditLogCard", () => {
  // The dials lead the card rather than hiding in a disclosure: a reader who
  // came to find one change narrows first, and a closed "Filters" summary made
  // that a hunt.
  it("keeps the filters in view above a table of changes, in one card", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => page([auditEntry])),
    );
    render(<AuditLogCard />);
    const action = await screen.findByText("Updated");

    const who = screen.getByLabelText("Who");
    const card = who.closest("section");
    expect(card).not.toBeNull();
    expect(who.closest("details")).toBeNull();
    expect(action.closest("section")).toBe(card);
    expect(card).toContainElement(
      screen.getByRole("heading", { level: 2, name: "Changes" }),
    );
    for (const dial of ["When", "Action", "Record type", "Record"]) {
      expect(screen.getByLabelText(dial)).toBeInTheDocument();
    }
    for (const column of ["When", "Who", "What", "Record"]) {
      expect(
        screen.getByRole("columnheader", { name: column }),
      ).toBeInTheDocument();
    }
  });

  it("names a record without a page by its type in words and a short id, never its table name", async () => {
    const scheduled = {
      ...auditEntry,
      id: "al-sys",
      actor_type: "system",
      actor_id: "system:scheduler",
      passport_id: null,
      on_behalf_of: null,
      action: "erase",
      entity_type: "forecast_capture_status",
      entity_id: "01a0fbe7-3c1d-7e2a-9b44-5d6e7f809a1b",
    };
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => page([scheduled])),
    );
    render(<AuditLogCard />);

    expect(
      await screen.findByText("Forecast capture status"),
    ).toBeInTheDocument();
    expect(screen.getByText("01a0fbe7")).toBeInTheDocument();
    expect(screen.queryByText(/forecast_capture_status/)).toBeNull();
    expect(screen.queryByText(/01a0fbe7-3c1d/)).toBeNull();
    // Destroying data reads in the danger tone; a routine write does not.
    expect(screen.getByText("Erased").closest(".badge")).toHaveClass(
      "badge-danger",
    );
  });

  it("narrows the request to the filters, keeping the page size and dropping the cursor", async () => {
    const user = steppedClock();
    const backend = auditLogBackend(() => page([auditEntry]));
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    expect(auditLogUrls(backend)[0]).toContain("limit=20");

    await pickOption(user, screen.getByLabelText("Action"), "Exported");
    await pickOption(user, screen.getByLabelText("Record type"), "Contact");
    await user.type(screen.getByLabelText("Actor ID"), "agent:sdr");
    await user.type(screen.getByLabelText("Bulk change ID"), "batch-7");

    // The card debounces what it asks, so the narrowed request exists only once
    // the debounce has elapsed, and that is stepped rather than waited out.
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
    await waitFor(() => {
      const latest = auditLogUrls(backend).at(-1) ?? "";
      expect(latest).toContain("actor=agent%3Asdr");
      expect(latest).toContain("action=export");
      expect(latest).toContain("entity_type=contact");
      expect(latest).toContain("batch_id=batch-7");
    });
    const latest = auditLogUrls(backend).at(-1) ?? "";
    expect(latest).toContain("limit=20");
    // A filter change is a new question, so the keyset chain starts over.
    expect(latest).not.toContain("cursor=");
  });

  it("says the log is empty in words that say what will fill it", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => page([])),
    );
    render(<AuditLogCard />);
    expect(
      await screen.findByText(
        "Changes appear here as soon as anyone creates or edits a record.",
      ),
    ).toBeInTheDocument();
  });

  it("tells a narrowed empty list apart from an empty log, and offers the way back", async () => {
    const user = steppedClock();
    const backend = auditLogBackend((url) =>
      page(url.includes("action=") ? [] : [auditEntry]),
    );
    vi.stubGlobal("fetch", backend);
    render(<AuditLogCard />);
    await screen.findByText("Updated");

    await pickOption(user, screen.getByLabelText("Action"), "Exported");
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
    expect(
      await screen.findByText("No changes match these filters"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
    expect(await screen.findByText("Updated")).toBeInTheDocument();
  });

  it("offers a retry when the log fails to load, and keeps the filters", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => ({
        status: 500,
        body: { title: "Upstream is down" },
      })),
    );
    render(<AuditLogCard />);
    expect(
      await screen.findByRole("button", { name: "Retry" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Could not load this view. Reload the page."),
    ).toBeInTheDocument();
    // A failed page must not take the controls that could ask a different
    // question with it.
    expect(screen.getByLabelText("Who")).toBeInTheDocument();
  });

  it("keeps the before/after diff and the attribution trail out of the table until a change is opened", async () => {
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => page([auditEntry])),
    );
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    expect(screen.queryByText("new")).toBeNull();
    expect(screen.queryByText("qualified")).toBeNull();
    expect(screen.queryByText("pp-9")).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Open details for this change" }),
    );
    const drawer = await screen.findByRole("dialog");
    expect(drawer).toHaveTextContent("new");
    expect(drawer).toHaveTextContent("qualified");
    expect(drawer).toHaveTextContent("pp-9");
    expect(drawer).toHaveTextContent("role:admin");
  });

  it("renders a non-scalar before/after value as its JSON string, not [object Object]", async () => {
    const objectValuedEntry = {
      ...auditEntry,
      id: "al-2",
      before: { address: { city: "Berlin" } },
      after: { address: { city: "Munich" } },
    };
    vi.stubGlobal(
      "fetch",
      auditLogBackend(() => page([objectValuedEntry])),
    );
    const user = userEvent.setup();
    render(<AuditLogCard />);
    await screen.findByText("Updated");
    await user.click(
      screen.getByRole("button", { name: "Open details for this change" }),
    );
    expect(await screen.findByText('{"city":"Berlin"}')).toBeTruthy();
    expect(screen.getByText('{"city":"Munich"}')).toBeTruthy();
    // The changed field reads as a word, not as its column name.
    expect(screen.getByText("Address")).toBeTruthy();
    expect(screen.queryByText("[object Object]")).toBeNull();
  });
});

// The When dial cuts days on the company clock the table reads on. At 23:30Z
// on 1 October it is already 2 October in Berlin, so "Today" starts at Berlin
// midnight, 22:00Z, rather than at UTC midnight two hours earlier.
describe("auditWhenRange", () => {
  const now = new Date("2026-10-01T23:30:00Z");
  const zone = "Europe/Berlin";
  const noCustom = { from: "", to: "" };

  it("starts Today at the company's midnight", () => {
    expect(auditWhenRange("today", noCustom, now, zone)).toEqual({
      from: "2026-10-01T22:00:00.000Z",
    });
  });

  it("counts the last seven days including today", () => {
    expect(auditWhenRange("last7", noCustom, now, zone)).toEqual({
      from: "2026-09-25T22:00:00.000Z",
    });
  });

  it("reads a custom range inclusive of the whole last day", () => {
    expect(
      auditWhenRange(
        "custom",
        { from: "2026-10-02", to: "2026-10-02" },
        now,
        zone,
      ),
    ).toEqual({
      from: "2026-10-01T22:00:00.000Z",
      to: "2026-10-02T21:59:59.999Z",
    });
  });

  it("asks for no range at any time", () => {
    expect(auditWhenRange("any", noCustom, now, zone)).toEqual({});
  });
});
