/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { SettingsScreen, settingsAddress } from "./settings";
import { jsonResponse, render } from "./settings.testkit";

// The Agents entry: the governed tool inventory an MCP client is shown, and the
// passports that scope it. Minting one has a file of its own
// (`settings-passports.test.tsx`); what is held here is the console — which
// tools an operator reads, which credential filters them, and the per-row revoke
// that has to reach BOTH cards, because they share the ["passports"] read.

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

// AS-2: the per-row Revoke kill-switch. A dedicated backend so the DELETE
// call can be asserted precisely, and a second passport is served already
// revoked to prove the button never shows on a row that's already dead.
function passportsBackend(opts: { onDelete?: (id: string) => void }) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (/\/passports\/[^/]+$/.test(url) && method === "DELETE") {
      const id = url.split("/passports/")[1];
      opts.onDelete?.(id);
      return new Response(null, { status: 204 });
    }
    if (url.includes("/passports")) {
      return jsonResponse({
        data: [
          {
            id: "pp-1",
            label: "Scout",
            scopes: ["read"],
            created_at: "2026-07-01T08:00:00Z",
            expires_at: null,
            revoked_at: null,
          },
          {
            id: "pp-2",
            label: "Retired",
            scopes: ["read"],
            created_at: "2026-06-01T08:00:00Z",
            expires_at: null,
            revoked_at: "2026-07-02T08:00:00Z",
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

// The governed tool console (IT-1): the /agent-tools inventory renders
// alongside an empty /passports list, so no row is dimmed and the
// egress badge shows only on the tool that reaches outside the workspace.
function agentToolsBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (url.includes("/agent-tools")) {
      return jsonResponse({
        data: [
          {
            name: "search_records",
            title: "Search records",
            description:
              'Find contacts, companies, deals, leads and projects by name. (Governance: runs immediately; requires passport scope "read".)',
            required_scope: "read",
            tier: "auto_execute",
            egress: false,
          },
          {
            name: "send_email",
            title: "Send email",
            description:
              'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
            required_scope: "send",
            tier: "confirmation_required",
            egress: true,
          },
        ],
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

describe("AgentToolsCard", () => {
  it("shows the egress badge only on the tool that reaches outside the workspace", async () => {
    vi.stubGlobal("fetch", agentToolsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);

    const sendRow = await screen.findByTestId("tool-send_email");
    const searchRow = screen.getByTestId("tool-search_records");
    expect(within(sendRow).getByText("External access")).toBeTruthy();
    expect(within(searchRow).queryByText("External access")).toBeNull();
  });

  // What an agent selects on is the served description, governance clause
  // included, so the row carries all of it: one line until it is opened.
  it("names each tool by its title, with its key and the text an agent selects it by", async () => {
    vi.stubGlobal("fetch", agentToolsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);

    const sendRow = await screen.findByTestId("tool-send_email");
    expect(within(sendRow).getByText("Send email")).toBeTruthy();
    expect(within(sendRow).getByText("send_email").tagName).toBe("CODE");
    expect(
      within(sendRow).getByText(/Governance: a human approves every call/),
    ).toBeTruthy();
    // The scope reads as the permission a human granted, never its wire token.
    expect(within(sendRow).getByText("Send messages")).toBeTruthy();
    expect(within(sendRow).queryByText("send")).toBeNull();
  });

  it("opens a tool's full description from its row and from its chevron", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", agentToolsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);

    const sendRow = await screen.findByTestId("tool-send_email");
    const chevron = within(sendRow).getByRole("button", {
      name: "Full description of Send email",
    });
    const description = within(sendRow).getByText(/Put a mail on the wire/);
    expect(chevron).toHaveAttribute("aria-expanded", "false");
    expect(chevron).toHaveAttribute("aria-controls", description.id);
    expect(description).toHaveClass("tools-description-clamped");

    await user.click(within(sendRow).getByText("Send email"));
    expect(chevron).toHaveAttribute("aria-expanded", "true");
    expect(description).not.toHaveClass("tools-description-clamped");

    await user.click(chevron);
    expect(chevron).toHaveAttribute("aria-expanded", "false");
    expect(description).toHaveClass("tools-description-clamped");
  });

  it("searches titles, keys and descriptions, and says when nothing matches", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", agentToolsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);
    const search = await screen.findByRole("searchbox", {
      name: "Search tools",
    });
    const shown = () =>
      ["search_records", "send_email"].filter((name) =>
        screen.queryByTestId(`tool-${name}`),
      );

    await user.type(search, "Send");
    expect(shown()).toEqual(["send_email"]);

    await user.clear(search);
    await user.type(search, "search_rec");
    expect(shown()).toEqual(["search_records"]);

    await user.clear(search);
    await user.type(search, "real recipient");
    expect(shown()).toEqual(["send_email"]);

    await user.clear(search);
    await user.type(search, "invoice");
    expect(shown()).toEqual([]);
    expect(screen.getByText("No tools match this search.")).toBeTruthy();
  });
});

// Both /passports and /agent-tools served together so the passport
// selector's filtering and the reachability computation can be exercised
// against the same fixture: one live passport, one revoked, and one
// scope-free tool alongside a scoped one the live passport doesn't cover.
function agentToolsWithPassportsBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (url.includes("/passports")) {
      return jsonResponse({
        data: [
          {
            id: "pp-1",
            label: "Scout",
            scopes: ["read"],
            created_at: "2026-07-01T08:00:00Z",
            expires_at: null,
            revoked_at: null,
          },
          {
            id: "pp-2",
            label: "Retired",
            scopes: ["read"],
            created_at: "2026-06-01T08:00:00Z",
            expires_at: null,
            revoked_at: "2026-07-02T08:00:00Z",
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.includes("/agent-tools")) {
      return jsonResponse({
        data: [
          {
            name: "list_pipelines",
            title: "List pipelines and their stages",
            description:
              'Every pipeline with its live stages. (Governance: runs immediately; requires passport scope "read".)',
            required_scope: null,
            tier: "auto_execute",
            egress: false,
          },
          {
            name: "send_email",
            title: "Send email",
            description:
              'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
            required_scope: "send",
            tier: "confirmation_required",
            egress: true,
          },
        ],
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

describe("AgentToolsCard passport scoping", () => {
  it("excludes a revoked passport from the selector", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", agentToolsWithPassportsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);
    await screen.findByTestId("tool-list_pipelines");

    // The options only exist while the popup is open — the control renders no
    // listbox when closed — so reading what it offers means opening it first.
    await user.click(screen.getByLabelText("All passports"));
    const optionLabels = screen
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(optionLabels).toContain("Reachable by Scout");
    expect(optionLabels).not.toContain("Reachable by Retired");
  });

  it("keeps a scope-free tool reachable once a passport is selected", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", agentToolsWithPassportsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);
    const freeRow = await screen.findByTestId("tool-list_pipelines");

    const select = screen.getByLabelText("All passports");
    await pickOption(user, select, "Reachable by Scout");

    expect(within(freeRow).queryByText("Scope not granted")).toBeNull();
    const scopedRow = screen.getByTestId("tool-send_email");
    expect(within(scopedRow).getByText("Scope not granted")).toBeTruthy();
    expect(
      within(scopedRow).getByText("Send email").closest(".agents-ended"),
    ).toBeTruthy();
  });

  // The reason sits with the permission it is about, not on the tool's name.
  it("says an unreachable tool's scope is not granted, in its permission cell", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", agentToolsWithPassportsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);
    const scopedRow = await screen.findByTestId("tool-send_email");

    await pickOption(
      user,
      screen.getByLabelText("All passports"),
      "Reachable by Scout",
    );

    const reason = within(scopedRow).getByText("Scope not granted");
    expect(reason.closest("td")).toBe(
      within(scopedRow).getByText("Send messages").closest("td"),
    );
  });

  // A human who only ever connected an agent through the OAuth consent screen
  // has never minted a passport of their own — the row the connection issued
  // carries `connection`, so mintedPassports filters it out. The selector must
  // not render for a filtered-to-nothing list: PassportSelect would offer only
  // "All passports", a choice that is already the default and picks among
  // nothing.
  it("hides the passport selector for a human who has only ever connected agents", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse({
            user: { email: "ada@acme.test" },
            roles: ["admin"],
            teams: [],
          });
        }
        if (url.includes("/passports")) {
          return jsonResponse({
            data: [
              {
                id: "pp-connection",
                label: "oauth:dcr-client-id",
                scopes: ["read"],
                created_at: "2026-07-01T08:00:00Z",
                expires_at: "2036-08-01T08:00:00Z",
                revoked_at: null,
                connection: {
                  client_id: "dcr-client-id",
                  client_name: "Claude Code",
                  connected_at: "2026-07-01T08:00:00Z",
                  renewable: true,
                },
              },
            ],
            page: { next_cursor: null, has_more: false },
          });
        }
        if (url.includes("/agent-tools")) {
          return jsonResponse({
            data: [
              {
                name: "list_pipelines",
                title: "List pipelines and their stages",
                description:
                  'Every pipeline with its live stages. (Governance: runs immediately; requires passport scope "read".)',
                required_scope: null,
                tier: "auto_execute",
                egress: false,
              },
            ],
          });
        }
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      }),
    );
    render(<SettingsScreen route={settingsAddress("agents")} />);
    await screen.findByTestId("tool-list_pipelines");

    expect(screen.queryByLabelText("All passports")).toBeNull();
  });
});

// The tool console and the passport list share the ["passports"] read, so a
// revoke on one card refetches the other's options. This backend answers the
// second read honestly: the revoked passport comes back marked revoked.
function revocablePassportsBackend(onDelete?: (id: string) => void) {
  const revoked = new Set<string>();
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (/\/passports\/[^/]+$/.test(url) && method === "DELETE") {
      const id = url.split("/passports/")[1];
      onDelete?.(id);
      revoked.add(id);
      return new Response(null, { status: 204 });
    }
    if (url.includes("/passports")) {
      return jsonResponse({
        data: [
          {
            id: "pp-1",
            label: "Scout",
            scopes: ["read"],
            created_at: "2026-07-01T08:00:00Z",
            expires_at: null,
            revoked_at: revoked.has("pp-1") ? "2026-07-03T08:00:00Z" : null,
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.includes("/agent-tools")) {
      return jsonResponse({
        data: [
          {
            name: "send_email",
            title: "Send email",
            description:
              'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
            required_scope: "send",
            tier: "confirmation_required",
            egress: true,
          },
        ],
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

// Revoke sits in the row's menu, so reaching it is two presses.
async function openRevoke(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Actions for ${name}` }),
  );
  await user.click(screen.getByRole("button", { name: `Revoke ${name}` }));
  return screen.findByRole("dialog");
}

describe("PassportCard revoke", () => {
  // Revoking the passport the tools were filtered by leaves the selector on
  // "All passports", so the inventory must read unfiltered too.
  it("stops scoping the tool table to a passport revoked while it was selected", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", revocablePassportsBackend());
    render(<SettingsScreen route={settingsAddress("agents")} />);
    const scopedRow = await screen.findByTestId("tool-send_email");

    const select = screen.getByLabelText("All passports");
    await pickOption(user, select, "Reachable by Scout");
    expect(within(scopedRow).getByText("Scope not granted")).toBeTruthy();

    const dialog = await openRevoke(user, "Scout");
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    // The options exist only while the popup is open, and it re-renders as the
    // refetched passports arrive.
    await user.click(select);
    await waitFor(() =>
      expect(
        screen.getAllByRole("option").map((option) => option.textContent),
      ).toEqual(["All passports"]),
    );
    expect(within(scopedRow).queryByText("Scope not granted")).toBeNull();
  });

  it("revokes through a confirm, fires the DELETE with its id, and lands focus on the struck row", async () => {
    const user = userEvent.setup();
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      revocablePassportsBackend((id) => deleted.push(id)),
    );
    render(<SettingsScreen route={settingsAddress("agents")} />);

    const dialog = await openRevoke(user, "Scout");
    expect(deleted).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "Revoke" }));

    await waitFor(() => expect(deleted).toEqual(["pp-1"]));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const anchor = document.querySelector('[data-passport="pp-1"]');
    expect(anchor).toHaveFocus();
    expect(within(anchor as HTMLElement).getByText("Revoked")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Actions for Scout" }),
    ).toBeNull();
  });

  it("offers no verb on a passport already revoked", async () => {
    vi.stubGlobal("fetch", passportsBackend({}));
    render(<SettingsScreen route={settingsAddress("agents")} />);
    await screen.findByRole("button", { name: "Actions for Scout" });
    expect(
      screen.queryByRole("button", { name: "Actions for Retired" }),
    ).toBeNull();
  });

  it("cancelling the confirm deletes nothing", async () => {
    const user = userEvent.setup();
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      passportsBackend({ onDelete: (id) => deleted.push(id) }),
    );
    render(<SettingsScreen route={settingsAddress("agents")} />);

    const dialog = await openRevoke(user, "Scout");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(deleted).toEqual([]);
  });
});
