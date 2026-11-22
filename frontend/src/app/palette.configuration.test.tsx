/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SettingName } from "../features/settings/configurationentries";
import { COMPANY_CONTEXT_ON } from "../features/settings/settingspages.fixtures";
import { LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { CommandPalette, useBuiltinCommands } from "./palette";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const SETTING_NAMES: readonly SettingName[] = [
  {
    key: "capture.auto_enrich_daily_cap",
    env: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
  },
  { key: "http.port", env: "MARGINCE_HTTP_PORT" },
];

const CONFIGURATION_PATH = "/installation/configuration";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function urlOf(input: RequestInfo | URL): string {
  return String(input instanceof Request ? input.url : input);
}

function Probe() {
  return (
    <CommandPalette open onClose={() => {}} commands={useBuiltinCommands()} />
  );
}

// The reader's grants answer /me, the settings answer Configuration, and every
// other read (the record search) finds nothing.
function mount(allow: GrantSpec) {
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = urlOf(input);
    if (url.endsWith(CONFIGURATION_PATH)) {
      return jsonResponse(SETTING_NAMES);
    }
    if (url.endsWith("/company/context/capabilities")) {
      return jsonResponse(COMPANY_CONTEXT_ON);
    }
    if (url.endsWith("/me")) {
      return jsonResponse(meFixture({ roles: ["ops"], allow }));
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Probe />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetch;
}

const INSTALLATION_WRITER: GrantSpec = {
  installation_settings: ["read", "update"],
};

describe("the palette, by a setting's name", () => {
  it.each([
    ["capture.auto_enrich_daily_cap", "capture.auto_enrich_daily_cap"],
    ["MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP", "capture.auto_enrich_daily_cap"],
    ["MARGINCE_", undefined],
  ])("opens Configuration filtered to %j", async (query, named) => {
    const user = userEvent.setup();
    mount(INSTALLATION_WRITER);
    await user.type(screen.getByRole("combobox"), query);

    const row = await screen.findByRole("option", { name: /^Configuration/ });
    expect(row.querySelector(".sub")?.textContent).toBe(named);
    await user.click(row);
    expect(window.location.hash).toBe(`#/settings/configuration?q=${query}`);
  });

  it("opens Configuration unfiltered when its own name matches", async () => {
    const user = userEvent.setup();
    mount(INSTALLATION_WRITER);
    await user.type(screen.getByRole("combobox"), "configuration");

    const rows = await screen.findAllByRole("option", {
      name: /^Configuration/,
    });
    expect(rows).toHaveLength(1);
    await user.click(rows[0]);
    expect(window.location.hash).toBe("#/settings/configuration");
  });

  // Matched inside a key, the row is a place the reader may mean. Enter keeps to
  // the results, and the arrows still reach it.
  it("lists Configuration for words inside a key but leaves Enter to the results", async () => {
    const user = userEvent.setup();
    mount(INSTALLATION_WRITER);
    const box = screen.getByRole("combobox");
    await user.type(box, "auto_enrich");
    await screen.findByRole("option", { name: /^Configuration/ });

    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/search/auto_enrich");

    await user.clear(box);
    await user.type(box, "auto_enrich");
    await screen.findByRole("option", { name: /^Configuration/ });
    await user.keyboard("{ArrowUp}{Enter}");
    expect(window.location.hash).toBe("#/settings/configuration?q=auto_enrich");
  });

  it("neither offers nor reads the settings for a reader without the grant", async () => {
    const user = userEvent.setup();
    // The witness: Mail capture proves /me has answered before the absence
    // below is read.
    const fetch = mount({ capture_settings: ["read"] });
    const box = screen.getByRole("combobox");
    await user.type(box, "capture");
    await screen.findByRole("option", { name: /^Mail capture/ });
    await user.clear(box);
    await user.type(box, "MARGINCE_HTTP_PORT");

    expect(screen.queryByRole("option", { name: /^Configuration/ })).toBeNull();
    expect(
      fetch.mock.calls.some(([input]) =>
        urlOf(input).endsWith(CONFIGURATION_PATH),
      ),
    ).toBe(false);
  });
});

describe("the palette, by half a word", () => {
  // "Disqualification reasons" is a keyword of the Leads settings page, and a
  // colleague's name starting "Ali" holds the same three letters.
  it("leaves a settings page whose keyword only holds the letters to the results", async () => {
    const user = userEvent.setup();
    mount({ custom_field: ["read"] });
    const box = screen.getByRole("combobox");
    await user.type(box, "qualif");
    await screen.findByRole("option", { name: /^Leads/ });
    await user.clear(box);
    await user.type(box, "ali");

    expect(screen.queryByRole("option", { name: /^Leads/ })).toBeNull();
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/search/ali");
  });
});
