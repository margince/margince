/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { isValidElement, type ReactElement } from "react";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { parseSource } from "../scripts/lib/source-tree";
import { addressedScreenCode, SCREEN_CODE } from "./App.screencode";
import * as screens from "./App.screens";
import { screenOfAddress, screenWithLocale } from "./App.views";
import { CompleteLocale } from "./i18n";
import { parseHash, routeHash, SCREENS } from "./lib/router";

// React's own lazy shape: `_init` throws the pending load, then returns the
// module's `default`. Read so a renamed export fails here, not on a first visit.
type LazyScreen = { _init: (payload: unknown) => unknown; _payload: unknown };

function isLazyScreen(value: unknown): value is LazyScreen {
  return (
    typeof value === "object" &&
    value !== null &&
    "$$typeof" in value &&
    value.$$typeof === Symbol.for("react.lazy") &&
    "_init" in value &&
    typeof value._init === "function" &&
    "_payload" in value
  );
}

async function loadedComponent(screen: LazyScreen): Promise<unknown> {
  for (;;) {
    try {
      return screen._init(screen._payload);
    } catch (pending) {
      if (!(pending instanceof Promise)) throw pending;
      await pending;
    }
  }
}

describe("the routed screens", () => {
  it("each loads its module and finds the component it names", async () => {
    const routeParts = [
      screens.DealsRoute({}),
      screens.DealsRoute({ id: "d1" }),
      screens.DealsRoute({ id: "d1", id2: "room" }),
      screens.ShareRoute({ id: "deal", id2: "d1" }),
    ].map((element) => (isValidElement(element) ? element.type : undefined));
    const lazies = [...Object.values(screens), ...routeParts].flatMap(
      (value) => (isLazyScreen(value) ? [value] : []),
    );
    expect(lazies.length).toBeGreaterThan(30);
    for (const screen of lazies)
      expect(await loadedComponent(screen)).toBeTruthy();
  }, 60_000);

  it("names a module that loads under every entry of the code map", async () => {
    for (const load of Object.values(SCREEN_CODE))
      expect(await load()).toBeTruthy();
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
    [
      "#/projects/p1",
      screens.ProjectScreen,
      { id: "p1", archivedHistory: screens.ArchivedRecordHistory },
    ],
    ["#/worklist/me", screens.WorklistRedirect, { opensOn: "me" }],
    ["#/analytics", screens.AnalyticsScreen, {}],
    ["#/filters/f1/v1", screens.FiltersScreen, { id: "f1", view: "v1" }],
    ["#/scheduled", screens.ScheduledSendsScreen, {}],
    ["#/legal-reviews/r1", screens.LegalReviewsRoute, { id: "r1" }],
    ["#/offers/o1", screens.OfferScreen, { id: "o1" }],
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
    ["#/reset-password", screens.ResetAddressSignedIn, {}],
    ["#/x/fork", screens.CustomRoute, { name: "fork" }],
    ["#/no-such-page", screens.UnknownAddress, {}],
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
    expect(screens.ShareRoute({ id: "deal" }).type).toBe(
      screens.UnknownAddress,
    );
  });

  it("answers a fork address the registry does not hold as not found", () => {
    expect(screens.CustomRoute({ name: "fork" })?.type).toBe(
      screens.UnknownAddress,
    );
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

// A call whose callee is the `import` keyword, however spaced; an `import()` type is no call.
function dynamicImports(node: ts.Node): number {
  const own =
    ts.isCallExpression(node) &&
    node.expression.kind === ts.SyntaxKind.ImportKeyword
      ? 1
      : 0;
  let total = own;
  ts.forEachChild(node, (child) => {
    total += dynamicImports(child);
  });
  return total;
}

describe("the screen code bootstrap starts", () => {
  it("is named by the screen-code map alone, never by a second import", () => {
    const path = join(__dirname, "App.screens.tsx");
    expect(dynamicImports(parseSource(path, readFileSync(path, "utf8")))).toBe(
      0,
    );
  });

  it.each([
    ['const x = import ("./x");', 1],
    ["const x = import(name);", 1],
    ['type X = typeof import("./x");', 0],
  ])("counts the dynamic imports in %s", (text, count) => {
    expect(dynamicImports(parseSource("planted.ts", text))).toBe(count);
  });

  it.each([
    ["#/home", SCREEN_CODE.brief],
    ["#/contacts/c1/notes", SCREEN_CODE.contactPage],
    ["#/contacts/import", SCREEN_CODE.contacts],
    ["#/contacts/new", SCREEN_CODE.contacts],
    ["#/projects/new", SCREEN_CODE.projects],
    ["#/projects/import", SCREEN_CODE.project],
    ["#/deals/d1/room", SCREEN_CODE.dealRoom],
    ["#/deals/new", SCREEN_CODE.deals],
    ["#/deals/new/room", SCREEN_CODE.deals],
    ["#/companies/c1", SCREEN_CODE.companies],
    ["#/settings/account", SCREEN_CODE.settings],
    ["#/offers/o1", SCREEN_CODE.offers],
    ["#/offers", undefined],
    ["#/no-such-page", undefined],
  ])("starts %s's code", (address, code) => {
    expect(addressedScreenCode(parseHash(address))).toBe(code);
  });
});
