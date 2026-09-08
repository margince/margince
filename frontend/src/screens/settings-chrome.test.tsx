/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as router from "../app/router";
import { PageTitle } from "../app/shell";
import { translate } from "../i18n";
import { SettingsScreen } from "./settings";
import {
  labelOf,
  readOn,
  render,
  renderNav,
  settingsBackend,
  settingsNavBackend,
} from "./settings.testkit";
import { useSettingsSection } from "./settingsnav";
import { settingsHref } from "./settingsrouting";

// What the settings chrome draws around a page: the rail’s own shape, the
// scope a page publishes to the shell, and the rail’s search box.
//
// WHICH pages a principal is offered at all is `settings-nav.test.tsx`.

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

describe("SettingsScreen page layout", () => {
  // These layout assertions run as an admin holding the catalog grants the
  // testkit's fixture carries, so every page under test is present. Which
  // principal sees which page is the visibility suite's subject, not this one's.
  beforeEach(() => {
    vi.stubGlobal("fetch", settingsBackend());
  });

  it("groups the nav by subject, Account current by default", async () => {
    // The RAIL, at a page address: this case is about the sidebar's own shape
    // and its current row, which the home address has no answer for.
    renderNav();
    // ONE navigation landmark in the chrome: the level names itself with a
    // heading inside it rather than opening a second `nav` beside the sidebar's
    // own. The name stands over the level's FIRST group, which is the one the
    // Overview row belongs to.
    const nav = screen.getByRole("navigation", { name: /primary navigation/i });
    const [name] = within(nav).getAllByRole("heading", { level: 2 });
    expect(name?.textContent).toBe("Settings");
    // The granted pages appear once the /me probe resolves the grant map.
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Fields" })).toBeTruthy(),
    );
    // The subject headings the level carries, after the one naming the level. A
    // group with no visible member is dropped rather than printed empty, so
    // this fixture's grants decide which of the seven appear — and the subjects
    // it does open are named in catalog order.
    expect(
      within(nav)
        .getAllByRole("heading", { level: 2 })
        .slice(1)
        .map((heading) => heading.textContent),
    ).toEqual(["You", "People", "Sales", "Governance"]);
    for (const label of [
      "Account",
      "Writing voice",
      "Agents",
      "Connections",
      "Members",
      "Fields",
      "Pipelines",
      "Privacy & retention",
    ]) {
      expect(screen.getByRole("link", { name: label })).toBeTruthy();
    }
    const account = screen.getByRole("link", { name: "Account" });
    expect(account.getAttribute("aria-current")).toBe("page");
    // Every row addresses the level's own depth. The group a page sits under is
    // no longer a segment in its address, so a page that changed group would
    // keep the link a reader bookmarked.
    expect(account.getAttribute("href")).toBe("#/settings/account");
    expect(
      screen.getByRole("link", { name: "Fields" }).getAttribute("href"),
    ).toBe("#/settings/fields");
  });

  it("renders only the active page's cards — the passport is off the Account page", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    // Scout lives on Agents; the default Account page must not render it.
    expect(screen.queryByText("Scout")).toBeNull();
  });

  it("renders the custom-field editor itself on the Fields page, never a door to it", async () => {
    render(<SettingsScreen route={settingsHref("fields")} />);
    // Visible once /me resolves the custom_field read grant.
    expect(
      await screen.findByRole("heading", { name: "Custom fields" }),
    ).toBeTruthy();
    // The editor IS the content now, so nothing on the page navigates to it.
    expect(screen.queryByRole("link", { name: /custom fields/i })).toBeNull();
  });

  it("renders the pipeline designer inline on its own page, never a door to it", async () => {
    render(<SettingsScreen route={settingsHref("pipelines")} />);
    expect(
      await screen.findByRole("heading", { name: "Pipelines" }),
    ).toBeTruthy();
    // A former standalone screen is inline content: the door-card that stood in
    // for it is gone rather than relabelled.
    const hrefs = screen
      .queryAllByRole("link")
      .map((link) => link.getAttribute("href"));
    expect(hrefs).not.toContain("#/pipelines");
  });

  it("renders the product and offer-template surfaces on one page, never doors to them", async () => {
    // The two priced surfaces share a page, so the claim spans both: each was a
    // standalone screen behind a door-card before, and both doors are gone.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("product") }),
    );
    render(<SettingsScreen route={settingsHref("products")} />);
    expect(
      await screen.findByRole("heading", { name: "Products" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Offer templates" }),
    ).toBeTruthy();
    const hrefs = screen
      .queryAllByRole("link")
      .map((link) => link.getAttribute("href"));
    expect(hrefs).not.toContain("#/products");
    expect(hrefs).not.toContain("#/offer-templates");
  });
});

// The catalog has declared a `scope` for every page since it was written, and
// nothing read it until now. Asserted through the REAL section rather than a
// hand-built one: `shell.test.tsx` proves PageTitle renders a scope it is
// handed — building the entry itself, since `fixtureSection` carries none — and
// this proves the settings level actually hands it one.
describe("the scope a settings page publishes", () => {
  function RealTitle({ hash }: Readonly<{ hash: string }>) {
    const route = router.parseHash(hash);
    return <PageTitle route={route} section={useSettingsSection(route)} />;
  }

  // `company` is deliberately not among these: its requirement ANDs the
  // company write with the `company_context` deployment flag, which the
  // default fixture leaves off, so the page is shut and has no heading to carry
  // a scope. The installation scope is covered by the pure catalog test instead.
  // `account` and `connections` are deliberately NOT here. Both open on
  // `always`, so their heading and badge render while `/me` is still in flight
  // — `findByText` would resolve on the loading paint, and a regression that
  // showed the badge during loading and dropped it once the snapshot arrived
  // would still pass. Their scope values are held by the pure catalog cases
  // instead, where there is no in-flight state to race.
  //
  // `pipelines` is safe for the opposite reason: it opens on a GRANT, every
  // grant predicate reads false against an unresolved snapshot, so its row
  // cannot appear until /me has answered.
  it.each([["pipelines", "settings.scope.workspace"]] as const)(
    "says whose state %s changes",
    async (page, key) => {
      vi.stubGlobal("fetch", settingsBackend());
      render(<RealTitle hash={`#/settings/${page}`} />);
      expect(await screen.findByText(translate("en", key))).toBeTruthy();
    },
  );
});

// The search box in the rail searches the READER'S pages, not the catalog.
//
// The box's own tests hand it a page list directly, so they cannot see which
// list the rail passes — a wiring that handed it SETTINGS_PAGES would offer a
// rep the audit log, and every one of those tests would still pass. This is the
// case that fails when that happens.
describe("the settings search in the rail", () => {
  it("offers a rep no page their own sidebar does not draw", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow: {} }));
    renderNav();

    // Waited on a row a resolved snapshot draws, so the empty result below is
    // about the grants rather than about a rail that has not loaded.
    await screen.findByRole("link", { name: labelOf("account") });
    await user.type(screen.getByRole("combobox"), "audit");

    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });

  // The control: the same word, one grant apart. Without it the case above
  // would pass against a search that finds nothing for anybody.
  it("offers the page to a reader who holds its grant", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["admin"], allow: readOn("audit_log") }),
    );
    renderNav();

    await screen.findByRole("link", { name: labelOf("audit") });
    await user.type(screen.getByRole("combobox"), "audit");

    expect(
      screen
        .queryAllByRole("option")
        .some((option) => option.textContent?.includes(labelOf("audit"))),
    ).toBe(true);
  });
});
