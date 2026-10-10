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
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { LeadHandlingCard } from "./leadvocab";
import { LeadDisqualifyReasonsCard } from "./leadvocab.reasons";
import { LeadSourcesCard } from "./leadvocab.sources";

// Settings › Data model: the lead vocabularies and the lead-handling posture.
// Every role reads them; the custom_field write verbs decide who may change
// them, and the cards disable rather than hide.

const ADMIN: GrantSpec = {
  custom_field: ["read", "create", "update", "delete"],
};
const READER: GrantSpec = { custom_field: ["read"] };

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
    } else if (url.includes("/lead-sources") && method === "GET") {
      body = {
        data: options.rows ?? [
          source("manual", "Created manually", { system: true, lead_count: 3 }),
          source("trade_show", "Trade show", { intent: "high" }),
        ],
        discovered: [{ key: "connector:apollo", lead_count: 7 }],
      };
    } else if (url.includes("/lead-sources")) {
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
});

describe("LeadSourcesCard", () => {
  it("names each source over its key, counts its leads and marks a built-in for a seat that may remove", async () => {
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    const manual = await screen.findByTestId("lead-source-manual");
    expect(within(manual).getByText("Created manually")).toBeTruthy();
    expect(within(manual).getByText("manual")).toBeTruthy();
    expect(within(manual).getByText("Built-in")).toBeTruthy();
    expect(within(manual).getByText("3")).toBeTruthy();
    // No always-open text box: a rename goes through the row's menu.
    expect(within(manual).queryByRole("textbox")).toBeNull();
  });

  it("folds each row onto a title line with its menu at the end, and the count keeps its unit", async () => {
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    const manual = await screen.findByTestId("lead-source-manual");
    const cells = [...manual.querySelectorAll("td")];
    expect(cells[0]?.getAttribute("data-fold")).toBe("title");
    expect(cells.at(-1)?.getAttribute("data-fold")).toBe("end");
    expect(within(manual).getByText("3 leads")).toBeTruthy();
  });

  it("renames through the menu's dialog, starting from the current label", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    await openMenu(user, "Trade show");
    await user.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = within(await screen.findByRole("dialog"));
    const field = dialog.getByLabelText(en["leadSources.labelField"]);
    expect(field).toHaveValue("Trade show");
    await user.clear(field);
    await user.type(field, "Messe{Enter}");
    await waitFor(() =>
      expect(
        wrote(calls, "PATCH", "/lead-sources/src-trade_show", {
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
        <LeadSourcesCard />
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
        wrote(calls, "PATCH", "/lead-sources/src-trade_show", {
          intent: "low",
        }),
      ).toBe(true);
      expect(
        wrote(calls, "PATCH", "/lead-sources/src-trade_show", {
          active: false,
        }),
      ).toBe(true);
    });
  });

  it("removes an unused source only after the confirmation", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadSourcesCard />
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
            c.url.endsWith("/lead-sources/src-trade_show"),
        ),
      ).toBe(true),
    );
    // The row and its menu go, so focus lands on the card's own verb.
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["leadSources.addOpen"] }),
      ).toHaveFocus(),
    );
  });

  it("refuses removing a built-in in words beside the verb", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend(ADMIN));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    await openMenu(user, "Created manually");
    const remove = screen.getByRole("button", { name: "Remove" });
    expect(remove).toBeDisabled();
    const reason = screen.getByText(en["leadSources.builtInKept"]);
    expect(remove.getAttribute("aria-describedby")).toContain(reason.id);
  });

  it("adds a source through the header verb's dialog and adopts a discovered value", async () => {
    const user = userEvent.setup();
    const calls: Call[] = [];
    vi.stubGlobal("fetch", backend(ADMIN, calls));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    await user.click(await screen.findByRole("button", { name: "New source" }));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByTestId("lead-source-new-label"), "Webinar");
    await user.click(dialog.getByRole("button", { name: "Add source" }));
    await waitFor(() =>
      expect(
        wrote(calls, "POST", "/lead-sources", {
          label: "Webinar",
          intent: "neutral",
        }),
      ).toBe(true),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const found = screen.getByTestId("lead-source-discovered-connector:apollo");
    expect(within(found).getByText("connector:apollo")).toBeTruthy();
    await user.click(
      within(found).getByRole("button", { name: "Add to list" }),
    );
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "POST" &&
            (c.body as { key?: string }).key === "connector:apollo",
        ),
      ).toBe(true),
    );
  });

  it("leaves every control inert for a reader and says why", async () => {
    vi.stubGlobal("fetch", backend(READER));
    render(
      <Providers>
        <LeadSourcesCard />
      </Providers>,
    );
    const toggle = await screen.findByRole("switch", {
      name: "Trade show is active",
    });
    expect(toggle).toBeDisabled();
    expect(screen.getByText(en["leadSources.readOnlyTitle"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["leadSources.addOpen"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: en["table.rowActions"].replace("{name}", "Trade show"),
      }),
    ).toBeNull();
    // Built-in changes nothing a reader can do, so it is not said.
    expect(screen.queryByText("Built-in")).toBeNull();
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
  [LeadSourcesCard, "1 lead uses this source.", "2 leads use this source."],
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

  it("refuses the flip for a reader and says why", async () => {
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
    expect(screen.getByText(en["leadSources.readOnly"])).toBeTruthy();
  });
});
