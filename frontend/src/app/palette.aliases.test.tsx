/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { COMPANY_CONTEXT_ON } from "../features/settings/settingspages.fixtures";
import { settingsSearch } from "../features/settings/settingssearch";
import { LocaleProvider, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { RbacAction } from "../lib/accessgrants";
import { analyticsContextOptions } from "../lib/analyticscontext";
import { companyContextCapabilitiesQueryKey } from "../lib/companycontextreach";
import {
  type CapabilityExpression,
  SETTINGS_PAGES,
  type SettingsPageId,
} from "../lib/settingscatalog";
import { useShortcutHandler } from "../lib/shortcuts";
import {
  type GrantSpec,
  meFixture,
  READS_DESTINATIONS,
  READS_RECORDS,
} from "../testing/mefixture";
import { analyticsContextFixture } from "../testing/reporting.fixtures";
import {
  type Command,
  CommandPalette,
  matchingCommands,
  useBuiltinCommands,
} from "./palette";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

// What a reader types for a page, one thought per phrase. Several are words the
// product once printed and no longer does, so a reader who learned them still lands.
const PHRASES: readonly (readonly [string, SettingsPageId])[] = [
  ["password", "account"],
  ["profile", "account"],
  ["working hours", "meetings"],
  ["writing style", "voice"],
  ["tone", "voice"],
  ["api key", "agents"],
  ["passport", "agents"],
  ["digest", "notifications"],
  ["mailbox", "connections"],
  ["capture log", "connections"],
  ["fx", "company"],
  ["fiscal year", "company"],
  ["oauth app", "authentication"],
  ["sign-in", "authentication"],
  ["sso", "authentication"],
  ["invite", "members"],
  ["license", "seats"],
  ["deal stages", "deals"],
  ["lead sources", "leads"],
  ["custom-fields", "records"],
  ["data model", "records"],
  ["labels", "records"],
  ["price list", "products"],
  ["rate card", "products"],
  ["offer-templates", "products"],
  ["email capture", "capture"],
  ["webhook", "integrations"],
  ["handbook", "knowledge"],
  ["csv", "import"],
  ["embeddings", "models"],
  ["triggers", "automations"],
  ["spend", "usage"],
  ["logs", "model-calls"],
  ["gdpr", "privacy"],
  ["law", "jurisdiction"],
  ["trail", "audit"],
  ["reindex", "system-health"],
  ["plugins", "extensions"],
  ["delete everything", "reset"],
  // The name of one setting on the page, which no alias carries.
  ["data retention", "system-health"],
  ["date format", "company"],
  ["sync interval", "capture"],
  ["two-step", "account"],
  ["timezone", "account"],
  ["dark mode", "account"],
  ["dark", "account"],
];

// Seats and license cannot change what anyone pays.
const BILLING_WORDS = ["billing", "plan", "subscription"];

// Every grant the catalog asks for, so every page is open to this reader.
function everyCatalogGrant(): GrantSpec {
  const allow: Record<string, RbacAction[]> = {};
  const walk = (expression: CapabilityExpression) => {
    if (expression.kind === "grant") {
      allow[expression.object] = [
        ...(allow[expression.object] ?? []),
        expression.action,
      ];
    }
    if (expression.kind === "any" || expression.kind === "all") {
      expression.of.forEach(walk);
    }
  };
  for (const page of SETTINGS_PAGES) {
    walk(page.requires);
    walk(page.changes);
    // A tab can ask for a grant its page does not, as System health's
    // configuration tab does.
    for (const tab of "tabs" in page ? page.tabs : []) {
      if ("requires" in tab) walk(tab.requires);
      if ("changes" in tab) walk(tab.changes);
    }
  }
  return allow;
}

async function adminCommands(): Promise<readonly Command[]> {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  });
  client.setQueryData(["me"], {
    ...meFixture({
      roles: ["admin"],
      allow: { ...READS_RECORDS, ...everyCatalogGrant() },
    }),
    data_reset_available: true,
  });
  client.setQueryData(companyContextCapabilitiesQueryKey, COMPANY_CONTEXT_ON);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const view = renderHook(() => useBuiltinCommands(), { wrapper });
  await waitFor(() =>
    expect(
      view.result.current.filter((command) =>
        command.id.startsWith("screen:settings-"),
      ),
    ).toHaveLength(SETTINGS_PAGES.length),
  );
  return view.result.current;
}

const t = (key: MessageKey) => translate("en", key);

function paletteFinds(commands: readonly Command[], phrase: string) {
  return matchingCommands(commands, phrase).map((command) => command.id);
}

function searchFinds(phrase: string): SettingsPageId[] {
  return settingsSearch(phrase, SETTINGS_PAGES, t, {
    seesCompanyContext: true,
  }).map((hit) => hit.page.id);
}

describe("the palette and the settings search", () => {
  it.each(PHRASES)("both send %j to %s", async (phrase, page) => {
    const commands = await adminCommands();
    expect(paletteFinds(commands, phrase)).toContain(`screen:settings-${page}`);
    expect(searchFinds(phrase)).toContain(page);
  });

  it.each(BILLING_WORDS)(
    "neither sends %j to Seats and license",
    async (word) => {
      const commands = await adminCommands();
      expect(paletteFinds(commands, word)).not.toContain(
        "screen:settings-seats",
      );
      expect(searchFinds(word)).not.toContain("seats");
    },
  );
});

// What a reader types for a screen or for the shortcuts, and where it lands.
const COMMAND_PHRASES: readonly (readonly [string, string])[] = [
  ["reports", "screen:analytics"],
  ["forecast", "screen:analytics"],
  ["shortcuts", "action:shortcuts"],
  ["keyboard", "action:shortcuts"],
  ["help", "action:shortcuts"],
];

describe("the palette's own commands", () => {
  it.each(COMMAND_PHRASES)("send %j to %s", async (phrase, id) => {
    const commands = await adminCommands();
    expect(paletteFinds(commands, phrase)).toContain(id);
  });

  it("opens the keyboard shortcuts as the ? key does", async () => {
    const commands = await adminCommands();
    const opened = vi.fn();
    renderHook(() => useShortcutHandler("shortcuts", opened));
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) =>
        (input instanceof Request ? input.url : String(input)).endsWith(
          "/installation/configuration",
        )
          ? Response.json([])
          : Response.json({
              data: [],
              page: { next_cursor: null, has_more: false },
            }),
      ),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <CommandPalette open onClose={onClose} commands={[...commands]} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await user.type(screen.getByRole("searchbox"), "keyboard shortcuts");
    await user.keyboard("{Enter}");
    expect(onClose).toHaveBeenCalled();
    expect(opened).toHaveBeenCalledTimes(1);
    expect(window.location.hash).toBe("");
  });
});

describe("a settings page in the palette", () => {
  it("is a setting on every page, never a screen", async () => {
    const pages = (await adminCommands()).filter((command) =>
      command.id.startsWith("screen:settings-"),
    );
    expect(pages.length).toBeGreaterThan(0);
    expect(pages.map((command) => command.type)).toEqual(
      pages.map(() => "setting"),
    );
  });

  // The settings tab and the rail row carry one name, so only the badge tells
  // the two rows apart.
  it.each([
    ["Deals", "#/deals"],
    ["Leads", "#/leads"],
  ])(
    "answers %s with the list screen first and the settings page under its own badge",
    async (typed, hash) => {
      const commands = await adminCommands();
      vi.stubGlobal(
        "fetch",
        vi.fn(async (input: RequestInfo | URL) =>
          (input instanceof Request ? input.url : String(input)).endsWith(
            "/installation/configuration",
          )
            ? Response.json([])
            : Response.json({
                data: [],
                page: { next_cursor: null, has_more: false },
              }),
        ),
      );
      const user = userEvent.setup();
      render(
        <QueryClientProvider client={new QueryClient()}>
          <LocaleProvider initial="en">
            <CommandPalette open onClose={() => {}} commands={[...commands]} />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      await user.type(screen.getByRole("searchbox"), typed);
      const named = screen
        .getAllByRole("button")
        .map((row) => row.textContent)
        .filter((text) => text?.startsWith(typed));
      expect(named.slice(0, 2)).toEqual([`${typed}Screen`, `${typed}Setting`]);
      await user.keyboard("{Enter}");
      expect(window.location.hash).toBe(hash);
    },
  );
});

describe("the Progress screen in the palette", () => {
  type AnalyticsContext = components["schemas"]["AnalyticsContext"];

  function progressLabel(frame: AnalyticsContext): string | undefined {
    const client = new QueryClient();
    client.setQueryData(["me"], meFixture({ allow: READS_DESTINATIONS }));
    client.setQueryData(analyticsContextOptions.queryKey, frame);
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    );
    const view = renderHook(() => useBuiltinCommands(), { wrapper });
    return view.result.current.find(
      (command) => command.id === "screen:progress",
    )?.label;
  }

  // The row a reader reads in the sidebar is the row they look for here.
  it("carries the sidebar's name for a manager who reads a team", () => {
    const team = { kind: "team", id: "t1", label: "Sales DACH" } as const;
    expect(progressLabel(analyticsContextFixture(team, [team]))).toBe(
      t("progress.teamTitle"),
    );
  });

  it("carries the sidebar's name for a reader of their own figures", () => {
    const own = { kind: "owner", id: "u1", label: "Alex Brandt" } as const;
    expect(progressLabel(analyticsContextFixture(own, [own]))).toBe(
      t("nav.myProgress"),
    );
  });
});

describe("the palette's matcher", () => {
  const company: Command = {
    id: "screen:settings-company",
    label: "Hồ sơ công ty",
    keywords: ["Múi giờ báo cáo", "Khoảng đồng bộ (giây)"],
    type: "setting",
  };

  it.each(["mui gio bao cao", "bao cao gio", "dong bo", "ho so cong"])(
    "finds %p through every word and without its marks",
    (query) => {
      expect(matchingCommands([company], query)).toEqual([company]);
    },
  );

  it("drops a command that holds only some of the words", () => {
    expect(matchingCommands([company], "mui kubernetes")).toEqual([]);
  });
});
