// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import type { AnalyticsScope } from "./analytics.context";
import { FirstRunNotice } from "./analytics.firstrun";
import {
  ownLensContext,
  render as renderScreen,
  reportsStub,
} from "./analytics.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const company: AnalyticsScope = { kind: "workspace", label: "Whole company" };
const mine: AnalyticsScope = { kind: "owner", id: "u-rep-1", label: "Riley" };
const team: AnalyticsScope = { kind: "team", id: "t-1", label: "Berlin" };

function notice(
  holdsDeals: boolean | undefined,
  scope: AnalyticsScope,
  onSelectScope = vi.fn(),
) {
  render(
    <StoryProviders>
      <FirstRunNotice
        holdsDeals={holdsDeals}
        scope={scope}
        allowedScopes={[company, team, mine]}
        onSelectScope={onSelectScope}
      />
    </StoryProviders>,
  );
  return onSelectScope;
}

it("points an empty own scope to Whole company", async () => {
  const select = notice(false, mine);
  expect(screen.getByText("You own no deals")).toBeInTheDocument();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Show Whole company" }));
  expect(select).toHaveBeenCalledWith(company);
});

it("names the first step when the whole company holds no deal", async () => {
  notice(false, company);
  expect(screen.getByText("No deals to report on yet")).toBeInTheDocument();
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Open Deals" }));
  expect(window.location.hash).toMatch(/^#\/deals/);
});

it("says an empty team holds no deal", () => {
  notice(false, team);
  expect(screen.getByText("Berlin holds no deals")).toBeInTheDocument();
});

it("says nothing when the scope holds deals or the server did not say", () => {
  notice(true, mine);
  notice(undefined, mine);
  expect(screen.queryByText("You own no deals")).toBeNull();
  expect(screen.queryByText("No deals to report on yet")).toBeNull();
});

it("tells Pipeline's empty own scope from the server flag, without asking for deals", async () => {
  const stub = reportsStub({
    dealRead: true,
    scopeHoldsDeals: false,
    context: {
      ...ownLensContext,
      allowed_scopes: [
        { kind: "workspace", label: "Whole company" },
        ...ownLensContext.allowed_scopes,
      ],
    },
  });
  vi.stubGlobal("fetch", stub);
  window.location.hash = "#/analytics/pipeline";
  renderScreen(<AnalyticsScreen />);
  expect(await screen.findByText("You own no deals")).toBeInTheDocument();
  const asked = stub.mock.calls.map(([input]) =>
    input instanceof Request ? input.url : String(input),
  );
  expect(asked.some((url) => /\/deals(\?|$)/.test(url))).toBe(false);
});

it("draws no notice on Pipeline when the scope holds deals", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      dealRead: true,
      scopeHoldsDeals: true,
      context: ownLensContext,
    }),
  );
  window.location.hash = "#/analytics/pipeline";
  renderScreen(<AnalyticsScreen />);
  await screen.findByText("Qualify");
  expect(screen.queryByText("You own no deals")).toBeNull();
});

it("says on Delivery that every readable project is covered", async () => {
  vi.stubGlobal("fetch", reportsStub({ dealRead: true }));
  window.location.hash = "#/analytics/delivery";
  renderScreen(<AnalyticsScreen />);
  expect(
    await screen.findByText("These numbers cover every project you can read."),
  ).toBeInTheDocument();
});
