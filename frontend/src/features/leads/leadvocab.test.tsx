/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
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
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { LeadHandlingCard } from "./leadvocab";
import { LeadOriginsCard } from "./leadvocab.origins";
import { LeadDisqualifyReasonsCard } from "./leadvocab.reasons";

// Settings › Data model: the lead vocabularies and the lead-handling posture.
// Every role reads them; the custom_field write verbs decide who may change
// them, and the cards disable rather than hide.

const ADMIN: GrantSpec = {
  custom_field: ["read", "create", "update", "delete"],
};
const READER: GrantSpec = { custom_field: ["read"] };
// Adds but may not edit: the page's read-only callout stays away, so the cards speak.
const ADDER: GrantSpec = { custom_field: ["read", "create"] };
const REMOVER: GrantSpec = { custom_field: ["read", "delete"] };

function source(
  key: string,
  label: string,
  extra: Partial<{
    system: boolean;
    lead_count: number;
    active: boolean;
    intent: string;
  }> = {},
) {
  return {
    id: `src-${key}`,
    key,
    label,
    intent: "neutral",
    sort_order: 10,
    active: true,
    system: false,
    lead_count: 0,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...extra,
  };
}

type Call = { url: string; method: string; body: unknown };

const CONFLICT = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

// `slaOn` seeds the stored first-response posture. The target box is inert
// while the target is off, so a case about the number starts with it on.
function backend(
  allow: GrantSpec,
  calls: Call[] = [],
  options: Readonly<{
    slaOn?: boolean;
    rows?: ReturnType<typeof source>[];
    refuseWrite?: boolean;
  }> = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    const raw = request ? await request.text() : String(init?.body ?? "");
    calls.push({ url, method, body: raw ? JSON.parse(raw) : undefined });
    let body: unknown;
    let status = 200;
    if (url.endsWith("/v1/me")) {
      body = meFixture({ allow });
    } else if (options.refuseWrite && method !== "GET") {
      body = CONFLICT;
      status = 409;
    } else if (method === "DELETE") {
      status = 204;
    } else if (url.includes("/lead-origins") && method === "GET") {
      body = {
        data: options.rows ?? [
          source("manual", "Created manually", { system: true, lead_count: 3 }),
          source("trade_show", "Trade show", { intent: "high" }),
        ],
      };
    } else if (url.includes("/lead-origins")) {
      body = source("trade_show", "Messe");
    } else if (url.includes("/lead-disqualify-reasons") && method === "GET") {
      body = {
        data: options.rows ?? [
          source("r1", "Bad timing", { system: true, lead_count: 2 }),
          source("r2", "Went quiet"),
        ],
      };
    } else if (url.includes("/lead-disqualify-reasons")) {
      body = source("r2", "Went quiet");
    } else if (url.endsWith("/leads/settings")) {
      body = {
        first_response_enabled: options.slaOn ?? false,
        first_response_target_minutes: 240,
      };
    }
    return new Response(status === 204 ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  });
}

function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

function wrote(calls: Call[], method: string, path: string, body: unknown) {
  return calls.some(
    (c) =>
      c.method === method &&
      c.url.endsWith(path) &&
      JSON.stringify(c.body) === JSON.stringify(body),
  );
}

async function openMenu(
  user: ReturnType<typeof userEvent.setup>,
  label: string,
) {
  await user.click(
    await screen.findByRole("button", {
      name: en["table.rowActions"].replace("{name}", label),
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("LeadOriginsCard", () => {
  it("names each origin without its key, and counts its leads", async () => {
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    const manual = await screen.findByTestId("lead-origin-manual");
    expect(within(manual).getByText("Created manually")).toBeTruthy();
    expect(within(manual).queryByText("manual")).toBeNull();
    expect(within(manual).getByText("3")).toBeTruthy();
    // No always-open text box: a rename goes through the row's menu.
    expect(within(manual).queryByRole("textbox")).toBeNull();
    expect(
      screen.getAllByRole("columnheader", {
        name: en["leadSources.colActions"],
      }),
    ).not.toHaveLength(0);
  });

  it("folds each row onto a title line with its menu at the end, and the count keeps its unit", async () => {
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    const manual = await screen.findByTestId("lead-origin-manual");
    const cells = [...manual.querySelectorAll("td")];
    expect(cells[0]?.getAttribute("data-fold")).toBe("title");
    expect(cells.at(-1)?.getAttribute("data-fold")).toBe("end");
    // The switch rides the title line, so its row's name is what it turns on.
    const toggle = within(manual).getByRole("switch");
    expect(toggle.closest("td")?.getAttribute("data-fold")).toBe("end");
    expect(within(manual).getByText("3 leads")).toBeTruthy();
  });

  it("renames through the menu's dialog, starting from the current label", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await openMenu(user, "Trade show");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = within(await screen.findByRole("dialog"));
    const field = dialog.getByLabelText(en["leadSources.labelField"]);
    expect(field).toHaveValue("Trade show");
    expect(dialog.getByText("trade_show").tagName).toBe("CODE");
    await user.clear(field);
    await user.type(field, "Messe{Enter}");
    await waitFor(() =>
      expect(
        wrote(calls, "PATCH", "/lead-origins/src-trade_show", {
          label: "Messe",
        }),
      ).toBe(true),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("re-weights through the intent select and flips the active switch, one PATCH each", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await user.click(
      await screen.findByRole("combobox", { name: "Intent of Trade show" }),
    );
    await user.click(screen.getByRole("option", { name: "Low intent" }));
    await user.click(
      screen.getByRole("switch", { name: "Trade show is active" }),
    );
    await waitFor(() => {
      expect(
        wrote(calls, "PATCH", "/lead-origins/src-trade_show", {
          intent: "low",
        }),
      ).toBe(true);
      expect(
        wrote(calls, "PATCH", "/lead-origins/src-trade_show", {
          active: false,
        }),
      ).toBe(true);
    });
  });

  it("removes an unused origin only after the confirmation", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await openMenu(user, "Trade show");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const dialog = within(await screen.findByRole("dialog"));
    expect(calls.some((c) => c.method === "DELETE")).toBe(false);
    await user.click(dialog.getByRole("button", { name: "Remove" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "DELETE" &&
            c.url.endsWith("/lead-origins/src-trade_show"),
        ),
      ).toBe(true),
    );
    // The row and its menu go, so focus lands on the card's own verb.
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["leadOrigins.addOpen"] }),
      ).toHaveFocus(),
    );
  });

  it("hands focus back to the row a later Remove was cancelled from", async () => {
    const user = userEvent.setup();
    const rows = [source("webinar", "Webinar"), source("fair", "Fair")];
    vi.stubGlobal("fetch", backend(ADMIN, [], { rows }));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await openMenu(user, "Webinar");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Remove",
      }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["leadOrigins.addOpen"] }),
      ).toHaveFocus(),
    );
    await openMenu(user, "Fair");
    const fairTrigger = screen.getByRole("button", {
      name: en["table.rowActions"].replace("{name}", "Fair"),
    });
    const fairMenu = document.getElementById(
      fairTrigger.getAttribute("aria-controls") ?? "",
    );
    if (!fairMenu) throw new Error("the menu for Fair drew no items");
    await user.click(within(fairMenu).getByRole("button", { name: "Remove" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: en["create.cancel"],
      }),
    );
    await waitFor(() => expect(fairTrigger).toHaveFocus());
  });

  it("hands focus to the card's title when the seat may remove but not add", async () => {
    const user = userEvent.setup();
    const rows = [source("webinar", "Webinar")];
    vi.stubGlobal(
      "fetch",
      backend({ custom_field: ["read", "delete"] }, [], { rows }),
    );
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await openMenu(user, "Webinar");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Remove",
      }),
    );
    await waitFor(() =>
      expect(screen.getByText(en["leadOrigins.title"])).toHaveFocus(),
    );
  });

  it("refuses removing a built-in in words beside the verb", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await openMenu(user, "Created manually");
    const remove = screen.getByRole("button", { name: "Remove" });
    expect(remove).toBeDisabled();
    const reason = screen.getByText(en["leadOrigins.builtInKept"]);
    expect(remove.getAttribute("aria-describedby")).toContain(reason.id);
  });

  it("adds an origin through the header verb's dialog", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await user.click(await screen.findByRole("button", { name: "New origin" }));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByTestId("lead-origin-new-label"), "Webinar");
    await user.click(dialog.getByRole("button", { name: "Add origin" }));
    await waitFor(() =>
      expect(
        wrote(calls, "POST", "/lead-origins", {
          label: "Webinar",
          intent: "neutral",
        }),
      ).toBe(true),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("says a duplicate origin name on the field and keeps the dialog open", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN, [], { refuseWrite: true }));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["leadOrigins.addOpen"] }),
    );
    const dialog = within(screen.getByRole("dialog"));
    const field = dialog.getByTestId("lead-origin-new-label");
    await user.type(field, "Trade show{Enter}");
    expect(await dialog.findByText(en["leadOrigins.duplicate"])).toBeTruthy();
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(dialog.queryByText(CONFLICT.detail)).toBeNull();
  });

  it("leaves every control inert for a reader, and leaves the why to the page", async () => {
    vi.stubGlobal("fetch", backend(READER));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    const toggle = await screen.findByRole("switch", {
      name: "Trade show is active",
    });
    expect(toggle).toBeDisabled();
    expect(screen.queryByText(en["leadSources.removeOnlyTitle"])).toBeNull();
    expect(screen.queryByText(en["leadSources.addOnlyTitle"])).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["leadOrigins.addOpen"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["table.rowActions"].replace("{name}", "Trade show"),
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("columnheader", {
        name: en["leadSources.colActions"],
      }),
    ).toBeNull();
  });

  it("says why the switches are inert to a seat that may add but not edit", async () => {
    vi.stubGlobal("fetch", backend(ADDER));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    expect(
      await screen.findByRole("button", { name: en["leadOrigins.addOpen"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("switch", { name: "Trade show is active" }),
    ).toBeDisabled();
    expect(screen.getByText(en["leadSources.addOnlyTitle"])).toBeTruthy();
    expect(screen.queryByText(en["leadSources.removeOnlyTitle"])).toBeNull();
  });

  it("tells a seat that may only remove which half is theirs", async () => {
    vi.stubGlobal("fetch", backend(REMOVER));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    expect(
      await screen.findByText(en["leadSources.removeOnlyTitle"]),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: en["table.rowActions"].replace("{name}", "Trade show"),
      }),
    ).toBeTruthy();
    expect(screen.queryByText(en["leadSources.addOnlyTitle"])).toBeNull();
  });
});

describe("LeadDisqualifyReasonsCard", () => {
  it("lists reasons by name with no key, and refuses removing the built-in one in words", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    const timing = await screen.findByTestId("lead-reason-src-r1");
    expect(within(timing).getByText("Bad timing")).toBeTruthy();
    expect(within(timing).queryByText("r1")).toBeNull();
    await openMenu(user, "Bad timing");
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    expect(screen.getByText(en["leadReasons.builtInKept"])).toBeTruthy();
  });

  it("removes a reason and hands focus to the card's verb", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    await openMenu(user, "Went quiet");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const dialog = within(await screen.findByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: "Remove" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["leadReasons.newLabel"] }),
      ).toHaveFocus(),
    );
    expect(
      calls.some(
        (c) =>
          c.method === "DELETE" &&
          c.url.endsWith("/lead-disqualify-reasons/src-r2"),
      ),
    ).toBe(true);
  });

  it("adds a reason through the header verb's dialog", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["leadReasons.newLabel"] }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(
      dialog.getByLabelText(en["leadReasons.labelField"]),
      " No champion {Enter}",
    );
    await waitFor(() =>
      expect(
        wrote(calls, "POST", "/lead-disqualify-reasons", {
          label: "No champion",
        }),
      ).toBe(true),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("says a duplicate name on the field and keeps the dialog open", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN, [], { refuseWrite: true }));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    await openMenu(user, "Went quiet");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = within(await screen.findByRole("dialog"));
    const field = dialog.getByLabelText(en["leadReasons.labelField"]);
    await user.clear(field);
    await user.type(field, "Bad timing{Enter}");
    expect(await dialog.findByText(en["leadReasons.duplicate"])).toBeTruthy();
    expect(field.getAttribute("aria-invalid")).toBe("true");
    // A changed name is a new attempt, so the refusal leaves the field.
    await user.type(field, "!");
    expect(dialog.queryByText(en["leadReasons.duplicate"])).toBeNull();
  });

  it("flips a reason's switch through one PATCH", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    await user.click(
      await screen.findByRole("switch", { name: "Went quiet is active" }),
    );
    await waitFor(() =>
      expect(
        wrote(calls, "PATCH", "/lead-disqualify-reasons/src-r2", {
          active: false,
        }),
      ).toBe(true),
    );
  });
});

// A custom entry still in use explains itself in the reader's grammar.
const IN_USE = [
  source("webinar", "Webinar", { lead_count: 1 }),
  source("referral", "Referral", { lead_count: 2 }),
];

it.each([
  [LeadOriginsCard, "1 lead uses this origin.", "2 leads use this origin."],
  [
    LeadDisqualifyReasonsCard,
    "1 lead has this reason.",
    "2 leads have this reason.",
  ],
])(
  "counts the leads holding an entry it refuses to remove (%#)",
  async (Card, one, many) => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN, [], { rows: IN_USE }));
    render(
      <Providers>
        <Card />
      </Providers>,
    );
    const suffix = " Deactivate it instead.";
    await openMenu(user, "Webinar");
    expect(screen.getByText(one + suffix)).toBeTruthy();
    await user.keyboard("{Escape}");
    await openMenu(user, "Referral");
    expect(screen.getByText(many + suffix)).toBeTruthy();
  },
);

describe("LeadHandlingCard", () => {
  it("shows the target off by default, flips it through one PATCH, and keeps the minutes field inert while off", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadHandlingCard />
      </Providers>,
    );
    const toggle = await screen.findByTestId("lead-first-response-switch");
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    const minutes = screen.getByTestId(
      "lead-first-response-target",
    ) as HTMLInputElement;
    expect(minutes.disabled).toBe(true);
    expect(minutes.value).toBe("240");
    await user.click(toggle);
    await waitFor(() =>
      expect(
        wrote(calls, "PATCH", "/leads/settings", {
          first_response_enabled: true,
        }),
      ).toBe(true),
    );
  });

  it("refuses a target outside the server's bounds without writing, and says what it wants", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls, { slaOn: true }));
    render(
      <Providers>
        <LeadHandlingCard />
      </Providers>,
    );
    const minutes = (await screen.findByTestId(
      "lead-first-response-target",
    )) as HTMLInputElement;
    expect(minutes.disabled).toBe(false);
    await user.clear(minutes);
    await user.type(minutes, "2{Tab}");
    // Announced and attached to the control, so a reader who cannot see the row hears it.
    const refusal = await screen.findByRole("alert");
    expect(refusal.textContent).toContain("from 15 to 10,080");
    expect(minutes.getAttribute("aria-invalid")).toBe("true");
    expect(minutes.getAttribute("aria-describedby")).toContain(refusal.id);
    expect(
      calls.some(
        (c) =>
          c.method === "PATCH" &&
          JSON.stringify(c.body).includes("first_response_target_minutes"),
      ),
    ).toBe(false);
  });

  it("refuses the flip for a reader and leaves the why to the page", async () => {
    vi.stubGlobal("fetch", backend(READER));
    render(
      <Providers>
        <LeadHandlingCard />
      </Providers>,
    );
    const toggle = (await screen.findByTestId(
      "lead-first-response-switch",
    )) as HTMLButtonElement;
    expect(toggle.disabled).toBe(true);
    expect(screen.queryByText(en["leadHandling.readOnly"])).toBeNull();
  });

  it("refuses the flip with its reason to a seat that may add but not edit", async () => {
    vi.stubGlobal("fetch", backend(ADDER));
    render(
      <Providers>
        <LeadHandlingCard />
      </Providers>,
    );
    const toggle = (await screen.findByTestId(
      "lead-first-response-switch",
    )) as HTMLButtonElement;
    expect(toggle.disabled).toBe(true);
    // The minutes row says the same, so neither control is dimmed without a reason.
    expect(screen.getAllByText(en["leadHandling.readOnly"])).toHaveLength(2);
    expect(
      screen.getByTestId("lead-first-response-target"),
    ).toHaveAccessibleDescription(new RegExp(en["leadHandling.readOnly"]));
  });
});

describe("a refused write names the card's notice on the control that sent it", () => {
  const notSaved = expect.stringContaining(en["leadSources.notSaved"]);

  it("describes the refused intent select, and neither the switch beside it nor another row", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN, [], { refuseWrite: true }));
    render(
      <Providers>
        <LeadOriginsCard />
      </Providers>,
    );
    const intent = await screen.findByRole("combobox", {
      name: "Intent of Trade show",
    });
    expect(intent).not.toHaveAttribute("aria-describedby");
    await user.click(intent);
    await user.click(screen.getByRole("option", { name: "Low intent" }));
    await screen.findByText(en["leadSources.notSaved"]);
    expect(intent).toHaveAccessibleDescription(notSaved);
    expect(
      screen.getByRole("switch", { name: "Trade show is active" }),
    ).not.toHaveAttribute("aria-describedby");
    expect(
      screen.getByRole("combobox", { name: "Intent of Created manually" }),
    ).not.toHaveAttribute("aria-describedby");
  });

  it("describes the refused reason switch alone", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN, [], { refuseWrite: true }));
    render(
      <Providers>
        <LeadDisqualifyReasonsCard />
      </Providers>,
    );
    const refused = await screen.findByRole("switch", {
      name: "Went quiet is active",
    });
    expect(refused).not.toHaveAttribute("aria-describedby");
    await user.click(refused);
    await screen.findByText(en["leadSources.notSaved"]);
    expect(refused).toHaveAccessibleDescription(notSaved);
    expect(
      screen.getByRole("switch", { name: "Bad timing is active" }),
    ).not.toHaveAttribute("aria-describedby");
  });

  it("describes the refused first-response switch after its row, and not the minutes", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backend(ADMIN, [], { slaOn: true, refuseWrite: true }),
    );
    render(
      <Providers>
        <LeadHandlingCard />
      </Providers>,
    );
    const toggle = await screen.findByTestId("lead-first-response-switch");
    const minutes = screen.getByTestId("lead-first-response-target");
    const before = minutes.getAttribute("aria-describedby");
    await user.click(toggle);
    await screen.findByText(en["leadSources.notSaved"]);
    expect(toggle).toHaveAccessibleDescription(
      expect.stringMatching(
        new RegExp(
          `^${en["leadHandling.firstResponseHint"]}.*${en["leadSources.notSaved"]}`,
        ),
      ),
    );
    expect(minutes.getAttribute("aria-describedby")).toBe(before);
  });
});
