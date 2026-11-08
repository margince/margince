/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { SettingsScreen, settingsAddress } from "./settings";

// Where a passport may work, on the passport page: chosen when it is minted and
// shown in the list. It is changed later in a drawer that says what it gains or
// loses before it saves.

beforeEach(() => {
  pretendReaderZone(UTC_ZONE);
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  globalThis.location.hash = "";
});

const ADMIN_AREAS = [
  "business",
  "compliance",
  "capture",
  "workspace_settings",
  "admin_settings",
  "own_account",
];

const EVERYWHERE = {
  id: "pp-all",
  label: "Scout",
  scopes: ["read"],
  areas: { mode: "all", areas: [] },
  created_at: "2026-07-01T08:00:00Z",
  last_used_at: null,
  expires_at: null,
  revoked_at: null,
};

const SALES_ONLY = {
  ...EVERYWHERE,
  id: "pp-sales",
  label: "Night runner",
  areas: { mode: "custom", areas: ["business", "own_account"] },
};

type Sent = { method: string; path: string; body: unknown };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Records every write, so a test reads the body the page sent rather than a
// call order. The list answers the passports it was given each time.
function backend(
  passports: readonly Record<string, unknown>[],
  reachable: readonly string[] = ADMIN_AREAS,
) {
  const sent: Sent[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input));
    const method = request?.method ?? "GET";
    if (url.pathname.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (method !== "GET" && url.pathname.includes("/passports")) {
      const body: unknown = request ? await request.json() : null;
      sent.push({ method, path: url.pathname, body });
      if (method === "PUT") {
        return jsonResponse(body);
      }
      return jsonResponse({
        id: "pp-new",
        label: "Scout",
        scopes: ["read", "draft"],
        areas: { mode: "all", areas: [] },
        created_at: "2026-08-01T08:00:00Z",
        expires_at: null,
        revoked_at: null,
        token: "mgp_live_0f3a91c4",
      });
    }
    if (url.pathname.endsWith("/v1/passports")) {
      return jsonResponse({
        data: passports,
        api_base_url: "https://crm.example.test/v1",
        reachable_areas: reachable,
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
  vi.stubGlobal("fetch", fetch);
  return { sent, fetch };
}

function renderAgents() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SettingsScreen route={settingsAddress("agents")} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function passportsTable(): HTMLElement {
  return screen.getByRole("table", { name: "Your passports" });
}

function areasCell(name: string): HTMLElement {
  const table = passportsTable();
  const headers = within(table).getAllByRole("columnheader");
  const at = headers.findIndex((header) => header.textContent === "Areas");
  const row = within(table).getByText(name).closest("tr");
  if (!row || at < 0) {
    throw new Error(`no Areas cell for ${name}`);
  }
  const cell = within(row).getAllByRole("cell")[at];
  if (!cell) {
    throw new Error(`no Areas cell for ${name}`);
  }
  return cell;
}

async function openChangeAreas(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Actions for ${name}` }),
  );
  await user.click(
    await screen.findByRole("button", { name: `Change areas for ${name}` }),
  );
  return screen.findByRole("dialog");
}

describe("PassportCard: the Areas column", () => {
  it("shows All areas, or the names of the chosen areas", async () => {
    backend([EVERYWHERE, SALES_ONLY]);
    renderAgents();
    await screen.findByText("Scout");
    expect(areasCell("Scout")).toHaveTextContent("All areas");
    expect(areasCell("Night runner")).toHaveTextContent("Sales workMy account");
  });

  it("has no Areas column and no change verb for a role that reaches one area", async () => {
    backend([EVERYWHERE], ["business"]);
    const user = userEvent.setup();
    renderAgents();
    await screen.findByText("Scout");
    expect(
      within(passportsTable()).queryByRole("columnheader", { name: "Areas" }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: "Actions for Scout" }));
    expect(
      screen.queryByRole("button", { name: "Change areas for Scout" }),
    ).toBeNull();
  });

  it("offers no change of areas on a passport that has expired", async () => {
    backend([{ ...EVERYWHERE, expires_at: "2020-01-01T00:00:00Z" }]);
    const user = userEvent.setup();
    renderAgents();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Scout" }),
    );
    expect(
      screen.queryByRole("button", { name: "Change areas for Scout" }),
    ).toBeNull();
  });
});

describe("PassportCard: changing areas", () => {
  it("says what the passport loses and what an agent is told, then saves the choice", async () => {
    const { sent } = backend([EVERYWHERE]);
    const user = userEvent.setup();
    renderAgents();
    const drawer = await openChangeAreas(user, "Scout");
    const save = within(drawer).getByRole("button", { name: "Save areas" });
    expect(save).toBeDisabled();
    expect(within(drawer).queryByText(/What an agent is told/)).toBeNull();

    await user.click(
      within(drawer).getByRole("radio", { name: /Choose areas/ }),
    );
    await user.click(
      within(drawer).getByRole("checkbox", { name: /Admin settings/ }),
    );
    await user.click(
      within(drawer).getByRole("checkbox", { name: /Compliance/ }),
    );

    expect(
      within(drawer).getByText(
        "It will no longer work in: Compliance and privacy and Admin settings. Its next call there is refused.",
      ),
    ).toBeInTheDocument();
    expect(
      within(drawer).getByText(
        "This passport may not work in the “Compliance and privacy” area. Ask the user to turn that area on for this passport in Settings → AI assistants, under Your passports or Your connections.",
      ),
    ).toBeInTheDocument();

    await user.click(save);
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(sent).toEqual([
      {
        method: "PUT",
        path: "/v1/passports/pp-all/areas",
        body: {
          mode: "custom",
          areas: ["business", "capture", "workspace_settings", "own_account"],
        },
      },
    ]);
  });

  it("says what a passport gains when it goes back to all areas", async () => {
    const { sent } = backend([SALES_ONLY]);
    const user = userEvent.setup();
    renderAgents();
    const drawer = await openChangeAreas(user, "Night runner");
    expect(
      within(drawer).getByText(
        /may not work in the “Compliance and privacy” area/,
      ),
    ).toBeInTheDocument();

    await user.click(within(drawer).getByRole("radio", { name: /All areas/ }));
    expect(
      within(drawer).getByText(
        "It will also work in: Compliance and privacy, Email and calendar capture, Company settings and Admin settings.",
      ),
    ).toBeInTheDocument();
    await user.click(
      within(drawer).getByRole("button", { name: "Save areas" }),
    );
    await vi.waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]?.body).toEqual({ mode: "all", areas: [] });
  });

  it("refuses to save a choice of no areas", async () => {
    backend([SALES_ONLY]);
    const user = userEvent.setup();
    renderAgents();
    const drawer = await openChangeAreas(user, "Night runner");
    await user.click(
      within(drawer).getByRole("checkbox", { name: /Sales work/ }),
    );
    await user.click(
      within(drawer).getByRole("checkbox", { name: /My account/ }),
    );
    expect(
      within(drawer).getByRole("button", { name: "Save areas" }),
    ).toBeDisabled();
    expect(
      within(drawer).getByText("Tick at least one area."),
    ).toBeInTheDocument();
  });
});

describe("PassportCard: minting with areas", () => {
  async function openMint(user: ReturnType<typeof userEvent.setup>) {
    renderAgents();
    await user.click(
      await screen.findByRole("button", { name: "New passport" }),
    );
    return screen.findByRole("dialog");
  }

  it("starts at all areas and sends them", async () => {
    const { sent } = backend([]);
    const user = userEvent.setup();
    const dialog = await openMint(user);
    expect(
      within(dialog).getByRole("radio", { name: /All areas/ }),
    ).toBeChecked();
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );
    await within(dialog).findByText("mgp_live_0f3a91c4");
    expect(sent[0]?.body).toMatchObject({ areas: { mode: "all", areas: [] } });
  });

  it("sends the chosen areas", async () => {
    const { sent } = backend([]);
    const user = userEvent.setup();
    const dialog = await openMint(user);
    await user.click(
      within(dialog).getByRole("radio", { name: /Choose areas/ }),
    );
    for (const area of [
      /Compliance/,
      /Email and calendar capture/,
      /Company settings/,
      /Admin settings/,
    ]) {
      await user.click(within(dialog).getByRole("checkbox", { name: area }));
    }
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );
    await within(dialog).findByText("mgp_live_0f3a91c4");
    expect(sent[0]?.body).toMatchObject({
      areas: { mode: "custom", areas: ["business", "own_account"] },
    });
  });

  it("shows no area section to a role that reaches one area", async () => {
    backend([], ["business"]);
    const user = userEvent.setup();
    const dialog = await openMint(user);
    expect(
      within(dialog).queryByRole("radio", { name: /All areas/ }),
    ).toBeNull();
  });
});
