// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { settingsReach } from "../../lib/settingscatalog";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { newClient, renderWith } from "../../testing/shellharness";
import { SettingsHome } from "./settingshome";

// "Your access" on the settings home: what the reader reads, and whose records
// they may change. Every seat that holds a record kind's read reads all of it,
// so the row scope answers only the second question.

const READS_ALL: GrantSpec = {
  contact: ["read"],
  company: ["read"],
  lead: ["read"],
  deal: ["read"],
  project: ["read"],
};

const WRITES_OWN: GrantSpec = {
  contact: ["read", "create", "update"],
  company: ["read", "create", "update"],
  lead: ["read", "create", "update"],
  deal: ["read", "create", "update"],
  project: ["read", "create", "update"],
};

function mountHome(snapshot: ReturnType<typeof meFixture>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
  const client = newClient();
  client.setQueryData(["me"], snapshot);
  renderWith(client, <SettingsHome reach={settingsReach(snapshot)} />);
}

// The value a definition list gives one of its terms.
async function fact(term: string): Promise<string | null | undefined> {
  const label = await screen.findByText(term, { selector: "dt" });
  return label.nextElementSibling?.textContent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Your access", () => {
  it("tells a User they read every record and change their own", async () => {
    mountHome(
      meFixture({ roles: ["rep"], rowScope: "own", allow: WRITES_OWN }),
    );

    expect(await fact(en["settings.home.readLabel"])).toBe(
      en["settings.home.read.some"].replace(
        "{kinds}",
        "contacts, companies, deals, leads and projects",
      ),
    );
    expect(await fact(en["settings.home.reachLabel"])).toBe(
      en["settings.home.reach.own"],
    );
  });

  it("tells a Read-only member they change no records, though their scope is all", async () => {
    mountHome(
      meFixture({ roles: ["read_only"], rowScope: "all", allow: READS_ALL }),
    );

    expect(await fact(en["settings.home.reachLabel"])).toBe(
      en["settings.home.reach.none"],
    );
  });

  it("tells a read seat it changes no records, whatever its role grants", async () => {
    mountHome(
      meFixture({
        roles: ["admin"],
        seat: "read",
        rowScope: "all",
        allow: WRITES_OWN,
      }),
    );

    expect(await fact(en["settings.home.reachLabel"])).toBe(
      en["settings.home.reach.none"],
    );
  });
});
