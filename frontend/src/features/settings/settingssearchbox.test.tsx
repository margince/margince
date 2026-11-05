/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { companyContextCapabilitiesQueryKey } from "../../lib/companycontextreach";
import * as router from "../../lib/router";
import { SETTINGS_PAGES, type SettingsPage } from "../../lib/settingscatalog";
import { meFixture } from "../../testing/mefixture";
import {
  configurationQueryKey,
  type SettingName,
} from "./configurationentries";
import {
  COMPANY_CONTEXT_OFF,
  COMPANY_CONTEXT_ON,
} from "./settingspages.fixtures";
import { SettingsSearchBox } from "./settingssearchbox";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const SETTING_NAMES: readonly SettingName[] = [
  {
    key: "capture.auto_enrich_daily_cap",
    env: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
  },
  { key: "http.port", env: "MARGINCE_HTTP_PORT" },
];

// The readonly array the box takes, not the 28-element tuple `SETTINGS_PAGES`
// is: a case narrowing the reader's pages hands it a filtered subset. /me is
// seeded rather than fetched, so the reader's role is settled before typing.
// The installation's settings, seeded like /me; `null` leaves them unread.
function mount(
  pages: readonly SettingsPage[] = SETTINGS_PAGES,
  roles: string[] = [],
  capabilities = COMPANY_CONTEXT_ON,
  settings: readonly SettingName[] | null = SETTING_NAMES,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  });
  client.setQueryData(["me"], meFixture({ roles }));
  client.setQueryData(companyContextCapabilitiesQueryKey, capabilities);
  if (settings !== null) {
    client.setQueryData(configurationQueryKey, settings);
  }
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SettingsSearchBox pages={pages} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const box = () => screen.getByRole("combobox");
const options = () => screen.queryAllByRole("option");

describe("the settings search box", () => {
  it("offers nothing until something is typed", () => {
    mount();
    expect(options()).toHaveLength(0);
    expect(box().getAttribute("aria-expanded")).toBe("false");
  });

  it("offers the pages a word reaches", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "audit");

    expect(options().length).toBeGreaterThan(0);
    expect(
      options().some((option) => option.textContent?.includes("Audit log")),
    ).toBe(true);
    expect(box().getAttribute("aria-expanded")).toBe("true");
  });

  // The company-context card draws where `companyContextShown` says, and only
  // there does "vat" lead to Company profile. Ops would find no VAT on it.
  it.each([
    [["admin"], true],
    [["ops"], false],
  ] as const)(
    "sends %j to Company profile for vat: %s",
    async (roles, sent) => {
      const user = userEvent.setup();
      mount(SETTINGS_PAGES, [...roles]);
      await user.type(box(), "vat");

      expect(
        options().some((option) =>
          option.textContent?.includes("Company profile"),
        ),
      ).toBe(sent);
    },
  );

  it("keeps vat from Company profile while the rollout hides the card", async () => {
    const user = userEvent.setup();
    mount(SETTINGS_PAGES, ["admin"], COMPANY_CONTEXT_OFF);
    await user.type(box(), "vat");

    expect(
      options().some((option) =>
        option.textContent?.includes("Company profile"),
      ),
    ).toBe(false);
  });

  // The search is handed the reader's own pages, so it cannot offer one the
  // sidebar did not draw. Asserted through the CONTROL rather than only through
  // the pure function, because the wiring is where a widening would happen.
  it("cannot offer a page the reader was not given", async () => {
    const user = userEvent.setup();
    mount(SETTINGS_PAGES.filter((page) => page.group === "me"));
    await user.type(box(), "audit");

    expect(options()).toHaveLength(0);
    expect(screen.getByRole("status").textContent).toMatch(
      /Matching settings pages: 0/,
    );
  });

  // A word that matches nothing is SAID. A box that did nothing would leave a
  // reader wondering whether it was still thinking.
  it("says so when nothing matches", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "kubernetes");

    expect(options()).toHaveLength(0);
    expect(screen.getByText(/no settings page matches/i)).toBeTruthy();
  });

  it("moves the active option with the arrows, and wraps", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "e");
    const count = options().length;
    expect(count).toBeGreaterThan(1);

    await user.keyboard("{ArrowDown}");
    expect(options()[0]?.getAttribute("aria-selected")).toBe("true");
    expect(box().getAttribute("aria-activedescendant")).toBe(options()[0]?.id);

    // Up from the first wraps to the last rather than stopping: a reader
    // reaching past the top means the other end.
    await user.keyboard("{ArrowUp}");
    expect(options()[count - 1]?.getAttribute("aria-selected")).toBe("true");
  });

  it("opens the active option on Enter", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    await user.keyboard("{ArrowDown}{Enter}");

    expect(navigate).toHaveBeenCalledWith({ screen: "settings", id: "audit" });
  });

  it("opens nothing on an input method's Enter", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    await user.keyboard("{ArrowDown}");

    fireEvent.keyDown(box(), { key: "Enter", keyCode: 229 });

    expect(navigate).not.toHaveBeenCalled();
  });

  // Enter on an untouched query opens nothing: the first row is a candidate,
  // not a choice the reader has made.
  it("opens nothing on Enter before an option is chosen", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    await user.keyboard("{Enter}");

    expect(navigate).not.toHaveBeenCalled();
  });

  // Escape clears the query and KEEPS the focus. A reader who clears a search
  // is still searching; throwing them out would make them find the box again.
  it("clears on Escape and keeps the focus", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "audit");
    expect(options().length).toBeGreaterThan(0);

    await user.keyboard("{Escape}");
    expect(options()).toHaveLength(0);
    expect((box() as HTMLInputElement).value).toBe("");
    expect(document.activeElement).toBe(box());
  });

  // The phone drawer hosting the box closes on Escape only if the empty box
  // leaves the key unhandled.
  it("leaves Escape to its host once the box is empty", async () => {
    const user = userEvent.setup();
    mount();
    const seen: boolean[] = [];
    const record = (event: KeyboardEvent) => seen.push(event.defaultPrevented);
    globalThis.addEventListener("keydown", record);
    await user.type(box(), "audit");
    await user.keyboard("{Escape}");
    await user.keyboard("{Escape}");
    globalThis.removeEventListener("keydown", record);

    expect(seen.slice(-2)).toEqual([true, false]);
  });

  // Pointer and keyboard reach the same place, by the same route.
  it("opens a hit on click", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    // The option IS the anchor now — a link nested inside one would be a
    // second interactive element inside an interactive row.
    const hit = options().find((option) =>
      option.textContent?.includes("Audit log"),
    );
    await user.click(hit as HTMLElement);

    expect(navigate).toHaveBeenCalledWith({ screen: "settings", id: "audit" });
  });

  // Every hit is a real link, so it can be opened in a tab and copied like any
  // other row in the rail beside it.
  it("gives every hit an address of its own", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "audit");

    // The loop proves nothing over an empty list, which is what it would be
    // if the search had answered nothing.
    expect(options().length).toBeGreaterThan(0);
    for (const option of options()) {
      expect(option.getAttribute("href")).toMatch(/^\/#\/settings\/[a-z-]+$/);
    }
  });

  // A modified click is the reader asking the BROWSER for a new tab, and
  // preventDefault on it would silently take that away — which is why these
  // rows are anchors rather than buttons.
  it("leaves a cmd-click to the browser", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    const hit = options().find((option) =>
      option.textContent?.includes("Audit log"),
    );
    await user.keyboard("{Meta>}");
    await user.click(hit as HTMLElement);
    await user.keyboard("{/Meta}");

    // The SPA did not answer it; the href did.
    expect(navigate).not.toHaveBeenCalled();
  });

  // A no-result query still SHOWS a popup, so the combobox must say it is
  // expanded. It used to report collapsed while the "nothing matches" panel
  // stood open, which is a control describing itself wrongly to the one reader
  // who depends on the description.
  it("reports itself expanded whenever the panel is on screen", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "kubernetes");

    expect(screen.getByText(/no settings page matches/i)).toBeTruthy();
    expect(box().getAttribute("aria-expanded")).toBe("true");
  });

  // The rows are driven from the input by aria-activedescendant, so they must
  // not join the page's Tab sequence — Tab from the box goes to the next
  // control, not into the list.
  it("keeps the options out of the tab sequence", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "audit");

    for (const option of options()) {
      expect(option.getAttribute("tabindex")).toBe("-1");
    }
  });

  // A click elsewhere closes the list and KEEPS the query: a reader who looks
  // away has not abandoned what they typed.
  it("closes on a click outside without losing the query", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "audit");
    expect(options().length).toBeGreaterThan(0);

    await user.click(document.body);
    expect(options()).toHaveLength(0);
    expect((box() as HTMLInputElement).value).toBe("audit");

    // …and typing brings it back, rather than leaving the reader with a box
    // that has stopped answering.
    await user.type(box(), "x{Backspace}");
    expect(options().length).toBeGreaterThan(0);
  });

  // The query goes when the reader arrives: a list left standing over the page
  // they just opened is a list they have to dismiss before reading it.
  it("clears itself after opening a page", async () => {
    const user = userEvent.setup();
    vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "audit");
    await user.keyboard("{ArrowDown}{Enter}");

    expect((box() as HTMLInputElement).value).toBe("");
    expect(options()).toHaveLength(0);
  });
  it("names the setting a hit was found by, and opens its page", async () => {
    const user = userEvent.setup();
    const navigate = vi.spyOn(router, "navigate").mockImplementation(() => {});
    mount();
    await user.type(box(), "data retention");

    const index = options().findIndex((option) =>
      option.textContent?.startsWith("System health"),
    );
    expect(options()[index]?.querySelector(".t-caption")?.textContent).toBe(
      "Data retention · Installation",
    );
    await user.keyboard("{ArrowDown}".repeat(index + 1));
    await user.keyboard("{Enter}");
    expect(navigate).toHaveBeenCalledWith(
      {
        screen: "settings",
        id: "system-health",
        id2: "configuration",
      },
      new Map([["section", "operations.retention.label"]]),
    );
  });
});

describe("the settings search box, by a setting's name", () => {
  const configuration = () =>
    screen.queryByRole("option", { name: /^Configuration/ });

  it.each([
    ["capture.auto_enrich_daily_cap", "capture.auto_enrich_daily_cap"],
    ["MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP", "capture.auto_enrich_daily_cap"],
  ])(
    "offers Configuration for %j and opens it filtered",
    async (query, key) => {
      const user = userEvent.setup();
      const navigate = vi
        .spyOn(router, "navigate")
        .mockImplementation(() => {});
      mount();
      await user.type(box(), query);

      const hit = screen.getByRole("option", { name: /^Configuration/ });
      expect(hit.querySelector(".t-caption")?.textContent).toBe(
        `${key} · Installation`,
      );
      expect(hit.getAttribute("href")).toBe(
        `/#/settings/configuration?q=${query}`,
      );
      await user.click(hit);
      expect(navigate).toHaveBeenCalledWith(
        { screen: "settings", id: "configuration" },
        new Map([["q", query]]),
      );
    },
  );

  it("offers Configuration for the variable prefix", async () => {
    const user = userEvent.setup();
    mount();
    await user.type(box(), "MARGINCE_");

    expect(configuration()?.getAttribute("href")).toBe(
      "/#/settings/configuration?q=MARGINCE_",
    );
  });

  it("neither offers nor reads the settings for a reader without the grant", async () => {
    const fetch = vi.fn(async () => new Response("[]"));
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    mount(
      SETTINGS_PAGES.filter((page) => page.id !== "configuration"),
      [],
      COMPANY_CONTEXT_ON,
      null,
    );
    await user.type(box(), "MARGINCE_HTTP_PORT");

    expect(configuration()).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });
});
