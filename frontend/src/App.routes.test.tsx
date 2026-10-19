/** @vitest-environment happy-dom */
import { isValidElement, type ReactElement } from "react";
import { describe, expect, it } from "vitest";
import * as screens from "./App.screens";
import { screenOfAddress } from "./App.views";

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
    ["#/companies", screens.CompaniesScreen, {}],
    ["#/companies/c1", screens.CompanyScreen, { id: "c1" }],
    ["#/partners", screens.PartnersScreen, {}],
    ["#/leads", screens.LeadsScreen, {}],
    ["#/leads/l1", screens.LeadScreen, { id: "l1" }],
    ["#/deals/d1/room", screens.DealsRoute, { id: "d1", id2: "room" }],
    ["#/projects", screens.ProjectsScreen, {}],
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
    ["#/preferences/tok", screens.PreferenceCenterScreen, { token: "tok" }],
    [
      "#/unsubscribe/tok/news",
      screens.UnsubscribeScreen,
      { token: "tok", purpose: "news" },
    ],
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
