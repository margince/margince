/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { parseHash, routeHash } from "../app/router";
import { translate } from "../i18n";
import { SettingsScreen } from "./settings";
import {
  jsonResponse,
  readOn,
  render,
  renderNav,
  settingsBackend,
} from "./settings.testkit";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";
import { settingsHref, settingsRouteTarget } from "./settingsrouting";

// WHERE a settings page lives, and which addresses reach it. Every page is
// addressed flat — `#/settings/<page>` — so three things have to hold together,
// and each of them is a different kind of claim:
//
//   - the ADDRESS a caller mints for a page (settingsHref),
//   - the PAGE an address names, including the two legacy shapes still in
//     circulation: the `admin/` segment the old two-audience IA put in front of
//     half the pages, and the ids that were genuinely renamed
//     (settingsRouteTarget),
//   - and the HREF a nav row actually carries, which is the one a reader clicks.
//
// The first two are derived from the catalog rather than restated beside it: a
// table of expected addresses is a second source of truth, and a page added to
// the catalog would keep passing against it.

// What a bookmark written before the flat addresses landed still carries, and
// what that page is called now. `general` was the installation aggregate, and
// its company profile is the part the id was named for.
const LEGACY_ADMIN_ADDRESS = "#/settings/admin/general";
const CURRENT_ADDRESS = "#/settings/company";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  globalThis.location.hash = "";
});

describe("settingsHref — the address one page lives at", () => {
  it("addresses every catalog page flat, whatever group it sits in", () => {
    // Swept over the catalog, so a page moving between groups changes nothing
    // here — which is the point of the flat shape: depth is no longer a
    // property of the group, so a regrouping is not a broken bookmark.
    for (const page of SETTINGS_PAGES) {
      expect(routeHash(settingsHref(page.id))).toBe(`#/settings/${page.id}`);
    }
  });

  it("spells two of them out", () => {
    // The sweep above is derived and would pass against a catalog that had gone
    // wrong in the same direction as the code. These are the literal strings a
    // link in a channel carries, one from each side of the old audience split.
    expect(routeHash(settingsHref("audit"))).toBe("#/settings/audit");
    expect(routeHash(settingsHref("voice"))).toBe("#/settings/voice");
  });

  it("addresses the section itself when no page is named", () => {
    expect(routeHash(settingsHref())).toBe("#/settings");
  });
});

describe("settingsRouteTarget — which page an address names", () => {
  it("resolves the address the product mints, and calls it current", () => {
    expect(settingsRouteTarget(parseHash(CURRENT_ADDRESS))).toEqual({
      kind: "page",
      page: "company",
      legacy: false,
    });
  });

  it("still resolves the legacy admin address, and flags it as legacy", () => {
    // A bookmark, a pasted link and a docs page must not land nowhere because
    // the IA dropped a level of naming — and the flag is what stops the two
    // spellings both staying in circulation.
    expect(settingsRouteTarget(parseHash(LEGACY_ADMIN_ADDRESS))).toEqual({
      kind: "page",
      page: "company",
      legacy: true,
    });
  });

  it("resolves a renamed id addressed flat, and flags it as legacy", () => {
    // The shallow half of the same problem: `#/settings/general` is what every
    // link written before the admin segment existed says, and it names a page
    // that has since been split. It resolves and is rewritten, exactly as the
    // deep spelling is.
    expect(settingsRouteTarget(parseHash("#/settings/general"))).toEqual({
      kind: "page",
      page: "company",
      legacy: true,
    });
  });

  it("resolves a page whose id never changed without flagging it", () => {
    // Ten of the sixteen old ids are still canonical page ids. They are not
    // aliases, and reporting one as legacy would rewrite the address bar on
    // arrival for no reason.
    expect(settingsRouteTarget(parseHash("#/settings/voice"))).toEqual({
      kind: "page",
      page: "voice",
      legacy: false,
    });
  });

  it("resolves nothing for a personal page addressed through the admin segment", () => {
    // `#/settings/admin/voice` is not an address the product ever minted.
    // Answering it would give one page two live addresses, and the rewrite
    // below would then have nothing to move towards.
    expect(settingsRouteTarget(parseHash("#/settings/admin/voice"))).toEqual({
      kind: "unknown",
    });
  });

  it("tells an address nobody minted apart from the section itself", () => {
    // `unknown` is a real answer and distinct from `home`: a typo in a shared
    // link must not quietly land on a page the sender never meant to send.
    expect(settingsRouteTarget(parseHash("#/settings/nowhere"))).toEqual({
      kind: "unknown",
    });
    expect(settingsRouteTarget(parseHash("#/settings"))).toEqual({
      kind: "home",
    });
  });

  it("does not read an inherited property as a page", () => {
    // The ids come straight off the address bar, so the rename map is
    // prototype-free: on an ordinary object `#/settings/constructor` would
    // resolve to an inherited FUNCTION rather than to undefined, and be
    // reported as a renamed page whose `page` is not an id at all.
    expect(settingsRouteTarget(parseHash("#/settings/constructor"))).toEqual({
      kind: "unknown",
    });
    expect(settingsRouteTarget(parseHash("#/settings/toString"))).toEqual({
      kind: "unknown",
    });
  });

  it("resolves every address the catalog mints, and flags none of them", () => {
    // The round trip, over the whole catalog: mint the address, parse it back
    // the way the hash router does, and land on the page it was minted for. A
    // page the two functions disagree about fails here rather than as a blank
    // screen behind one row.
    for (const page of SETTINGS_PAGES) {
      expect(
        settingsRouteTarget(parseHash(routeHash(settingsHref(page.id)))),
      ).toEqual({ kind: "page", page: page.id, legacy: false });
    }
  });
});

// The label a row carries back to the page that published it, so the assertion
// below reads the catalog rather than a list of hrefs written out by hand.
const PAGE_BY_LABEL = new Map<string, SettingsPageId>(
  SETTINGS_PAGES.map((page) => [
    translate("en", `settings.tab.${page.id satisfies SettingsPageId}`),
    page.id,
  ]),
);

describe("the nav rows a reader actually clicks", () => {
  // The rail is the production wiring — the section this screen publishes,
  // rendered by the real level — so this is the href a reader gets, not the one
  // `settingsHref` would have produced. The row used to be computed by
  // different code, through the entry's group prefix, which is why it is still
  // worth asserting separately from the minting function above.
  it("links every row flat, whatever group heading it sits under", async () => {
    vi.stubGlobal("fetch", settingsBackend());
    renderNav();
    // The granted rows arrive with the /me answer, so waiting on one of them is
    // waiting for the level to be complete.
    await screen.findByRole("link", { name: "Fields" });
    const rows = screen
      .getAllByRole("link")
      .map((link) => ({
        label: link.textContent ?? "",
        href: link.getAttribute("href") ?? "",
      }))
      .filter((row) => row.href.startsWith("#/settings"));
    // A level that rendered nothing would pass every claim below it.
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      // Settings home is a row and not a page: it is the address with NO page
      // segment, so it has no entry in PAGE_BY_LABEL and its href is the bare
      // `#/settings`. Skipped by its HREF rather than its label, so a page that
      // ever went missing from the map still fails below.
      if (row.href === "#/settings") {
        continue;
      }
      const page = PAGE_BY_LABEL.get(row.label);
      if (!page) {
        throw new Error(`the level published a row for no page: ${row.label}`);
      }
      expect(row.href).toBe(`#/settings/${page}`);
    }
  });
});

// A reader who may open the page the legacy address names. The rewrite only
// happens on a page the reader can actually see — a fallback rewrites to where
// they LANDED, not to where they asked — so a fixture that cannot open `company`
// would make both cases below about the fallback instead of about the rewrite.
function companyReaderBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: ["admin"],
          // The UPDATE, which is what Company profile asks: the read is held by
          // every seat and no longer opens the page. These cases are about the
          // address rewrite, so they hold the grant that reaches the page.
          allow: {
            ...readOn("installation_settings"),
            installation_settings: ["read", "update"],
          },
        }),
      );
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

describe("a legacy address is answered and rewritten in place", () => {
  it("serves the page it names and moves the URL to the current address", async () => {
    vi.stubGlobal("fetch", companyReaderBackend());
    globalThis.location.hash = LEGACY_ADMIN_ADDRESS;
    // The address the reader came from is a real history entry, which is what
    // makes the count below mean anything.
    const entriesBefore = globalThis.history.length;

    render(<SettingsScreen route={parseHash(LEGACY_ADMIN_ADDRESS)} />);

    // They get the page they asked for — the rewrite is not a detour through
    // somewhere else.
    expect(
      await screen.findByRole("heading", { name: "Installation" }),
    ).toBeTruthy();
    await waitFor(() => expect(globalThis.location.hash).toBe(CURRENT_ADDRESS));
    // REPLACED, not pushed: a pushed redirect leaves the address that redirects
    // sitting one step back, so Back lands on it, it redirects again, and the one
    // key a reader has for getting out of things cannot get them out.
    expect(globalThis.history.length).toBe(entriesBefore);
  });

  it("leaves a current address alone", async () => {
    // The mirror case, and the reason the flag exists rather than a rewrite on
    // every render: an address that is already the current one must not be
    // navigated at all.
    vi.stubGlobal("fetch", companyReaderBackend());
    globalThis.location.hash = CURRENT_ADDRESS;
    const entriesBefore = globalThis.history.length;

    render(<SettingsScreen route={parseHash(CURRENT_ADDRESS)} />);

    expect(
      await screen.findByRole("heading", { name: "Installation" }),
    ).toBeTruthy();
    expect(globalThis.location.hash).toBe(CURRENT_ADDRESS);
    expect(globalThis.history.length).toBe(entriesBefore);
  });
});
