/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, renderSettings } from "../../testing/settings.testkit";

// The two entries one Connections tab became: the CONTACT's mailbox and network
// on Connections, the INSTALLATION's outside wiring on Integrations. Each page
// has to carry its own half and none of the other's — a relabelling would pass
// any claim that only ever looked at one of them.

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The installation's outside wiring sits on Integrations: one shared provider
// key and one set of outbound subscriptions. The personal mailbox and LinkedIn
// network that used to share the entry are on Connections, and neither page
// carries the other's cards.
function wiringSettingsBackend(opts: { roles: string[]; allow?: GrantSpec }) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      const me = meFixture({ roles: opts.roles, allow: opts.allow ?? {} });
      return jsonResponse({
        ...me,
        user: { ...me.user, email: "ada@acme.test" },
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

// The term of the Integrations predicate, granted wherever a case needs the
// entry to be OPEN — so an absent card on it can only mean the card is elsewhere.
//
// The WRITE, because that is what the page asks: every seeded role reads the
// object, and gating the page on the read put the installation's outside wiring
// in front of a rep who could only look at it. These cases are about what the
// page renders, so they hold the grant that reaches it.
const WIRING_WRITES: GrantSpec = {
  webhook_subscription: ["read", "create", "update"],
};

describe("SettingsScreen connections and integrations tabs", () => {
  // A retired id is what a bookmark still carries: the audit trail was an entry
  // of its own before it moved onto Privacy & retention, so `#/settings/audit` names
  // nothing. It has to land on the first entry this principal can see rather than
  // on a blank screen. The wiring reads are granted so Integrations is genuinely
  // open — a fallback that happens because an entry is hidden proves nothing
  // about a route id that no longer exists.
  it("shows the boundary when the route names a retired entry", async () => {
    vi.stubGlobal(
      "fetch",
      wiringSettingsBackend({ roles: ["admin"], allow: WIRING_WRITES }),
    );
    renderSettings("audit");
    // A page this reader may not open says so, and the address stays as typed.
    // It used to render Account and rewrite the URL to match, which left a
    // reader with no way to tell a shared link had gone somewhere else.
    expect(
      await screen.findByText(/no access to this settings page/i),
    ).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Profile" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Webhooks" })).toBeNull();
  });

  // The split, from both sides. One entry used to hold a rep's own mailbox and
  // the installation's webhooks together, which is why it could carry no honest
  // predicate: any gate on it took a personal task away from whoever it hid it
  // from. The two cases below are what makes the split real rather than a
  // relabelling — each page has to carry its own half and NOT the other's, and
  // the wiring reads are granted in both so an absent card can only mean the card
  // lives on the other entry.
  it("renders the personal connections on Connections and none of the installation's wiring", async () => {
    vi.stubGlobal(
      "fetch",
      wiringSettingsBackend({ roles: ["admin"], allow: WIRING_WRITES }),
    );
    renderSettings("connections");
    // Every surface here reads a per-user seam: the connector list is scoped to
    // the calling human server-side, and the LinkedIn panel reads /me.
    expect(
      await screen.findByRole("heading", {
        name: "Mail and calendar",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "LinkedIn network" }),
    ).toBeTruthy();
    // And nothing workspace-wide: a key everybody spends from, the
    // subscriptions everybody's writes fire and the company's one bot.
    for (const heading of ["Contact data", "Webhooks", "Telegram bot"]) {
      expect(screen.queryByRole("heading", { name: heading })).toBeNull();
    }
    // What the mailbox did with mail is Capture activity's, and the address in
    // outgoing links is a System health reading: neither is a connection.
    for (const heading of [
      "What your connections found",
      "Links in outgoing mail",
    ]) {
      expect(screen.queryByRole("heading", { name: heading })).toBeNull();
    }
  });

  it("renders what the connections found on Capture activity", async () => {
    vi.stubGlobal("fetch", wiringSettingsBackend({ roles: ["admin"] }));
    renderSettings("capture-activity");
    expect(
      await screen.findByRole("heading", {
        name: "What your connections found",
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Mail and calendar" }),
    ).toBeNull();
  });

  it("renders the installation's wiring on Integrations and none of the personal connections", async () => {
    vi.stubGlobal(
      "fetch",
      wiringSettingsBackend({ roles: ["admin"], allow: WIRING_WRITES }),
    );
    renderSettings("integrations");
    expect(
      await screen.findByRole("heading", { name: "Contact data" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Webhooks" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Telegram bot" })).toBeTruthy();
    for (const heading of ["Mail and calendar", "LinkedIn network"]) {
      expect(screen.queryByRole("heading", { name: heading })).toBeNull();
    }
  });
});
