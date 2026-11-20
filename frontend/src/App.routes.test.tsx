/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { isValidElement, type ReactElement } from "react";
import { describe, expect, it } from "vitest";
import { addressedScreenCode, SCREEN_CODE } from "./App.screencode";
import * as screens from "./App.screens";
import { screenOfAddress, screenWithLocale } from "./App.views";
import { CREATE_ID, IMPORT_ID, opensReservedPage } from "./app/nav";
import { CompleteLocale } from "./i18n";
import { parseHash, routeHash, SCREENS } from "./lib/router";

// Every routed screen is a lazy chunk, and a renamed export only fails when a
// reader first opens that page. Loading each chunk here fails the build first.
describe("the routed chunks", () => {
  it("each resolves to a component", async () => {
    expect(screens.ROUTE_CHUNKS.length).toBeGreaterThan(20);
    const modules = await Promise.all(
      screens.ROUTE_CHUNKS.map((load) => load()),
    );
    for (const module of modules) {
      const { default: component } = module as { default?: unknown };
      expect(component).toBeTruthy();
    }
  }, 60_000);
});

function viewOf(address: string): ReactElement<Record<string, unknown>> {
  const view = screenOfAddress(address);
  if (!isValidElement<Record<string, unknown>>(view)) {
    throw new Error(`${address} drew no element`);
  }
  return view;
}

// The dispatch from an address to what draws it. Each row names the
// component and the props the address must hand it.
describe("the address dispatch", () => {
  it.each([
    ["#/companies", screens.CompaniesScreen, { startCreating: false }],
    ["#/companies/new", screens.CompaniesScreen, { startCreating: true }],
    ["#/companies/c1", screens.CompanyScreen, { id: "c1" }],
    [
      "#/contacts",
      screens.ContactsScreen,
      { importing: false, startCreating: false },
    ],
    [
      "#/contacts/new",
      screens.ContactsScreen,
      { importing: false, startCreating: true },
    ],
    [
      "#/contacts/import",
      screens.ContactsScreen,
      { importing: true, startCreating: false },
    ],
    ["#/contacts/c1", screens.ContactPageV2, { id: "c1", tab: "overview" }],
    ["#/partners", screens.PartnersScreen, {}],
    ["#/leads", screens.LeadsScreen, { startCreating: false }],
    ["#/leads/new", screens.LeadsScreen, { startCreating: true }],
    ["#/leads/l1", screens.LeadScreen, { id: "l1" }],
    ["#/deals/d1/room", screens.DealsRoute, { id: "d1", id2: "room" }],
    ["#/projects", screens.ProjectsScreen, { startCreating: false }],
    ["#/projects/new", screens.ProjectsScreen, { startCreating: true }],
    ["#/projects/p1", screens.ProjectScreen, { id: "p1" }],
    ["#/worklist/me", screens.WorklistRedirect, { opensOn: "me" }],
    ["#/analytics", screens.AnalyticsScreen, {}],
    ["#/filters/f1/v1", screens.FiltersScreen, { id: "f1", view: "v1" }],
    ["#/scheduled", screens.ScheduledSendsScreen, {}],
    ["#/legal-reviews/r1", screens.LegalReviewsRoute, { id: "r1" }],
    ["#/offers/o1", screens.OfferScreen, { id: "o1" }],
    ["#/offers", screens.ScreenNotice, { messageKey: "screen.pending" }],
    ["#/tags/t1", screens.TagResultScreen, { tagID: "t1" }],
    ["#/lists/l1", screens.FiltersScreen, { list: "l1" }],
    ["#/share/deal/d1", screens.ShareRoute, { id: "deal", id2: "d1" }],
    ["#/client", screens.ClientSurfaceScreen, {}],
    ["#/book/host", screens.BookingScreen, { hostSlug: "host" }],
    [
      "#/book/host/return",
      screens.BookingScreen,
      { hostSlug: "host", page: "return" },
    ],
    ["#/preferences/tok", screens.PreferenceCenterScreen, { token: "tok" }],
    [
      "#/unsubscribe/tok/news",
      screens.UnsubscribeScreen,
      { token: "tok", purpose: "news" },
    ],
    ["#/unsubscribe/tok?lang=en", screens.UnsubscribeScreen, { token: "tok" }],
    ["#/confirm/tok", screens.ConfirmDetailsScreen, { token: "tok" }],
    ["#/reset-password", screens.ResetRoute, {}],
    ["#/x/fork", screens.CustomRoute, { name: "fork" }],
    [
      "#/no-such-page",
      screens.ScreenNotice,
      { messageKey: "shell.unknownPage" },
    ],
  ])("%s draws its screen", (address, component, props) => {
    const view = viewOf(address);
    expect(view.type).toBe(component);
    expect(view.props).toEqual(props);
  });
});

// The route parts choose between screens by the address alone, so their
// choice is read from the element they return, without a render.
describe("the route parts", () => {
  const drawn = (element: ReturnType<typeof screens.DealsRoute>) =>
    (element as ReactElement<Record<string, unknown>>).props;

  it("opens a deal, its room, the list, or the list with its form", () => {
    expect(drawn(screens.DealsRoute({ id: "d1" }))).toEqual({ id: "d1" });
    expect(drawn(screens.DealsRoute({ id: "d1", id2: "room" }))).toEqual({
      dealId: "d1",
    });
    expect(drawn(screens.DealsRoute({}))).toEqual({ startCreating: false });
    expect(drawn(screens.DealsRoute({ id: "new" }))).toEqual({
      startCreating: true,
    });
  });

  it("shares a record only when the address names both its type and id", () => {
    expect(drawn(screens.ShareRoute({ id: "deal", id2: "d1" }))).toEqual({
      recordType: "deal",
      recordId: "d1",
    });
    expect(drawn(screens.ShareRoute({ id: "deal" }))).toEqual({
      messageKey: "screen.pending",
    });
  });

  it("answers a fork address the registry does not hold as not found", () => {
    expect(drawn(screens.CustomRoute({ name: "fork" }))).toEqual({
      messageKey: "shell.unknownPage",
    });
  });
});

// The shell paints from the essential catalog; a screen's column waits for the
// complete one unless the screen draws from the essential catalog alone.
describe("the catalog each screen waits for", () => {
  const addresses = SCREENS.flatMap((screen) =>
    [undefined, "r1"].map((id) => routeHash({ screen, id })),
  );

  it.each(addresses)("%s", (address) => {
    const drawn = screenWithLocale(address);
    const essential = ["home", "analytics"].includes(parseHash(address).screen);
    expect(isValidElement(drawn) && drawn.type === CompleteLocale).toBe(
      !essential,
    );
  });
});

describe("the screen code bootstrap starts", () => {
  it("is named by the screen-code map alone, never by a second import", () => {
    const source = readFileSync(join(__dirname, "App.screens.tsx"), "utf8");
    expect(source).not.toMatch(/import\(/);
  });

  // A list address loads the list's module, so the split must be nav's.
  const reservedOrNot = [undefined, CREATE_ID, IMPORT_ID, "r1"];
  it.each(
    (["contacts", "projects"] as const).flatMap((screen) =>
      reservedOrNot.map((id) => ({ screen, id })),
    ),
  )("opens the list exactly where nav does: $screen/$id", (route) => {
    const list =
      route.screen === "contacts" ? SCREEN_CODE.contacts : SCREEN_CODE.projects;
    expect(addressedScreenCode(route) === list).toBe(
      route.id === undefined || opensReservedPage(route),
    );
  });

  it.each(reservedOrNot.map((id) => ({ screen: "deals" as const, id })))(
    "opens a deal room exactly where nav names a deal: $id",
    (route) => {
      expect(
        addressedScreenCode({ ...route, id2: "room" }) === SCREEN_CODE.dealRoom,
      ).toBe(route.id !== undefined && !opensReservedPage(route));
    },
  );

  it.each([
    ["#/home", SCREEN_CODE.brief],
    ["#/contacts/c1/notes", SCREEN_CODE.contactPage],
    ["#/deals/d1/room", SCREEN_CODE.dealRoom],
    ["#/deals/new", SCREEN_CODE.deals],
    ["#/companies/c1", SCREEN_CODE.companies],
    ["#/settings/account", SCREEN_CODE.settings],
    ["#/offers/o1", SCREEN_CODE.offers],
    ["#/offers", undefined],
    ["#/no-such-page", undefined],
  ])("starts %s's code", (address, code) => {
    expect(addressedScreenCode(parseHash(address))).toBe(code);
  });
});
