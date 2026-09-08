/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Route } from "../app/router";
import * as router from "../app/router";
import { SettingsRail } from "../app/shell";
import { translate } from "../i18n";
import { SettingsScreen } from "./settings";
import {
  expectSnapshotResolved,
  labelOf,
  readOn,
  render,
  renderHome,
  renderNav,
  renderSettings,
  settingsNavBackend,
} from "./settings.testkit";
import { settingsHref } from "./settingsrouting";

// Addressing a settings page the rail did not offer.
//
// The rail is about what a reader is OFFERED; this file is about what happens
// when they type an address anyway — a page they may not open, a page that does
// not exist, and a page that is theirs to read while the rail leaves it out.

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

// The access boundary, which replaced a silent fallback.
//
// The fallback was wrong in a way the reader could not see: a denied address
// rendered Account AND rewrote the URL to say Account, so somebody following a
// colleague's link had no evidence the link had gone anywhere else. They would
// report it broken; the sender would open it and find it worked.
describe("the settings access boundary", () => {
  it("tells a reader the page is not theirs, and leaves the address alone", async () => {
    const replaced: Route[] = [];
    vi.spyOn(router, "navigateReplacing").mockImplementation((route) => {
      replaced.push(route);
    });
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow: {} }));
    // Both halves, because the claim spans them: the SCREEN says the page is
    // not theirs, and the RAIL must not go on marking a page current beside it.
    render(
      <>
        <SettingsRail route={settingsHref("audit")} />
        <SettingsScreen route={settingsHref("audit")} />
      </>,
    );

    // The boundary is ALSO what renders while /me is in flight — every
    // capability predicate reads false until the snapshot lands — so finding it
    // proves nothing on its own. Waited on a row only a RESOLVED snapshot can
    // draw, and only then asserted the denial, which is what makes it a claim
    // about the grant rather than about the load.
    expect(
      await screen.findByRole("link", { name: labelOf("account") }),
    ).toBeTruthy();
    expect(
      screen.getByText(/this settings page is not yours to open/i),
    ).toBeTruthy();
    // Account's own content is what the fallback used to show here.
    expect(screen.queryByText("test@example.test")).toBeNull();
    // And the address is untouched, which is the whole affordance: the reader
    // can read what they asked for and quote it to somebody who holds it.
    expect(replaced).toEqual([]);
    // Nor does the CHROME claim a page. The sidebar used to mark Account
    // current beside a body saying "not yours" — half the false fallback,
    // living on in the rail.
    expect(
      screen
        .getByRole("link", { name: labelOf("account") })
        .getAttribute("aria-current"),
    ).toBeNull();
  });

  it("tells a reader an address names no page, which is a different fact", async () => {
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["admin"], allow: {} }));
    render(
      <SettingsScreen route={{ screen: "settings", id: "no-such-page" }} />,
    );

    expect(
      await screen.findByText(/no settings page has this address/i),
    ).toBeTruthy();
    // Not the denial: an admin holding every grant is refused nothing, and
    // telling them the page is "not theirs" would send them asking for a grant
    // that would not help.
    expect(screen.queryByText(/not yours to open/i)).toBeNull();
  });

  it("opens the page for a reader who does hold the grant", async () => {
    // The control: the same address, one grant apart. Without it the two cases
    // above would pass against a page nobody can ever open.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["admin"], allow: readOn("audit_log") }),
    );
    render(<SettingsScreen route={settingsHref("audit")} />);

    await waitFor(() =>
      expect(screen.queryByText(/not yours to open/i)).toBeNull(),
    );
    expect(
      screen.getByText(translate("en", "settings.tab.audit")),
    ).toBeTruthy();
  });
});

// The whole life of a page the rail does not carry, in one case.
//
// Each half was covered separately and the sequence was not, which is how a
// tidier rail could quietly become a permission change: the rail's own tests
// pass when a page is absent, and the reachability tests pass when it is
// present, so nothing failed if "absent from the rail" ever turned into
// "absent". A rep holding `pipeline:read` and no pipeline write is the case —
// they consult the pipeline vocabulary and cannot edit it.
describe("a page the rail does not carry is still the reader's to reach", () => {
  // Not `as const`: that freezes `roles` to a readonly tuple, which the fixture's
  // own `string[]` will not take. Vitest transpiles without typechecking, so
  // this only ever fails in `tsc -b` — which is the gate, not the test run.
  const consultingRep = { roles: ["rep"], allow: readOn("pipeline") };

  it("leaves it out of the rail", async () => {
    vi.stubGlobal("fetch", settingsNavBackend(consultingRep));
    renderNav();
    await screen.findByRole("link", { name: labelOf("account") });
    // Waited on a row a resolved snapshot draws, so this absence is about the
    // grant rather than about a rail that has not loaded.
    expect(
      screen.queryByRole("link", { name: labelOf("pipelines") }),
    ).toBeNull();
  });

  it("lists it on the settings home, under what you can look up", async () => {
    vi.stubGlobal("fetch", settingsNavBackend(consultingRep));
    renderHome();
    await expectSnapshotResolved();
    const heading = await screen.findByRole("heading", {
      name: translate("en", "settings.home.lookUp"),
    });
    const panel = heading.closest("section") ?? heading.parentElement;
    if (!panel) {
      throw new Error("the look-up panel has no container");
    }
    // The home row names the page, its scope and its subtitle in one accessible
    // name — that is the whole point of the row, so the match is on the label it
    // starts with rather than on the label alone.
    expect(
      within(panel).getByRole("link", {
        name: new RegExp(`^${labelOf("pipelines")}`),
      }),
    ).toBeTruthy();
  });

  it("finds it in the search", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", settingsNavBackend(consultingRep));
    renderNav();
    await screen.findByRole("link", { name: labelOf("account") });
    await user.type(screen.getByRole("combobox"), "pipeline");
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual([expect.stringContaining(labelOf("pipelines"))]);
  });

  it("opens on its own address, with its own heading and no boundary", async () => {
    vi.stubGlobal("fetch", settingsNavBackend(consultingRep));
    renderSettings("pipelines");
    // The page's own name in the chrome, not the section's. The heading comes
    // from the rail's rows, and this page has none — so a regression here reads
    // as a page called "Settings" rather than as a missing page.
    expect(
      await screen.findByRole("heading", { name: labelOf("pipelines") }),
    ).toBeTruthy();
    expect(
      screen.queryByText(translate("en", "settings.boundary.deniedTitle")),
    ).toBeNull();
  });

  it("says once that it is theirs to read and not to change", async () => {
    vi.stubGlobal("fetch", settingsNavBackend(consultingRep));
    renderSettings("pipelines");
    expect(
      await screen.findByText(translate("en", "settings.readOnlyPage")),
    ).toBeTruthy();
  });

  // The control for the case above: the same page, one grant apart. Without it
  // the banner assertion would pass against a page that always shows it.
  it("says nothing of the kind to a reader who can change it", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["rep"],
        allow: { pipeline: ["read", "create", "update"] },
      }),
    );
    renderSettings("pipelines");
    await screen.findByRole("heading", { name: labelOf("pipelines") });
    expect(
      screen.queryByText(translate("en", "settings.readOnlyPage")),
    ).toBeNull();
  });
});
