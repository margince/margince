/** @vitest-environment jsdom */
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "./api/schema";
import {
  memoryStorage,
  renderApp,
  sessionOnlyFetch,
} from "./testing/appharness";

// A tab inside a record is a VIEW of that record, and the app keys its routed
// subtree on what the address is ABOUT (app/router.tsx's routeIdentity) so that
// moving between views is a re-render rather than a remount. What that buys is
// visible: the record's header, its readings and the tab strip the reader just
// clicked keep their DOM nodes, so design-system/enter.css plays the arrival
// animation for the panel that actually changed and for nothing else.
//
// It is asserted here rather than in screens/personpage.test.tsx because the key
// lives above every screen: that suite mounts the page with a `tab` prop and can
// never see the router decide.
//
// KNOWN GAP, #3675: these two cases pass alone and fail when another App suite
// has run first. The person page then never leaves "Loading…" — the address is
// right and the record's name resolves, so the /360 read is what does not
// settle, and something above this file is holding a react-query cache across
// suites that `renderApp`'s fresh client does not clear. It is not this file's
// stubs: it reproduces on an unmodified tree. Until that is found, treat a
// failure here as the leak rather than as a broken identity key, and check by
// running this file alone.

type Person360 = components["schemas"]["Person360"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

// The smallest record that draws a header and a tab strip. A panel showing an
// error is as good a panel as one showing data here, so nothing else is stubbed.
function person(id: string, name: string): Person360 {
  return {
    as_of: "2026-08-13T09:00:00Z",
    person: { id, full_name: name, ...CAPTURED },
    sections_omitted: [],
    activities: { data: [], page: { has_more: false } },
    deal_roles: { data: [], page: { has_more: false } },
    profile_fields: [],
  };
}

// The session harness's stub, with the reads the record page cannot render
// without answered on top of it. Everything else still 503s into its own error
// state — this suite is about which DOM nodes survive a navigation.
//
// TWO reads, not one. The page asks for the person itself before it asks for the
// 360 projection, and a suite that answered only the second sat at "Loading…"
// forever: no header, no tab strip, and both cases failing on their first
// `waitFor` rather than on the node comparison they exist for. That is a whole
// screen this file cannot draw, so it is stubbed here rather than left to the
// harness — the harness deliberately 503s everything it is not asked about.
function personFetch() {
  const session = sessionOnlyFetch();
  return async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const whole = /\/v1\/people\/(p-\d+)\/360$/.exec(url);
    if (whole) {
      return json(person(whole[1], `Person ${whole[1]}`));
    }
    // The record itself, which the page reads for its header. Anchored to the
    // end so it cannot swallow /360, /brief or /consent — those keep 503ing
    // into panels of their own, which is what this suite wants.
    const bare = /\/v1\/people\/(p-\d+)$/.exec(url);
    if (bare) {
      return json({ id: bare[1], full_name: `Person ${bare[1]}`, ...CAPTURED });
    }
    return session(input);
  };
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// The one element every case below compares across a navigation: the RECORD's
// header, not the shell's top bar. It is the top of the page and the furthest
// thing from the panel that changed, so if it survives, everything between them
// did too.
function recordHead(): Element {
  const found = document.querySelector(".record-head");
  if (!found) {
    throw new Error("the record's header never rendered");
  }
  return found;
}

// The strip ITSELF, from the component that draws the tabs — not the slot the
// page chrome reserves for it.
//
// `.record-tabs` in composed.tsx is that slot: a wrapper the record head puts
// around whatever tabs it is handed. The tabs live inside it under
// `recordtabs.tsx`'s own classes, and a query for `.record-tabs button` matches
// nothing at all. This file asked for the wrapper and for buttons inside it, so
// it compared a node that survives everything and then failed to find a current
// tab — which is how both cases died on their first `waitFor` rather than on the
// identity comparison they exist for.
function tabStrip(): Element {
  const found = document.querySelector(".recordtabs-strip");
  if (!found) {
    throw new Error("the contact record's tab strip never rendered");
  }
  return found;
}

// Which tab the strip says is current. The tab arrives as a prop from the router,
// so this is the observable proof that the navigation reached the screen — and
// waiting on it is what makes the node comparisons below claims about a move
// that HAPPENED rather than one still in flight.
function currentTab(): string {
  const on = document.querySelector(
    '.recordtabs-strip button[aria-pressed="true"]',
  );
  if (!on) {
    throw new Error("no tab is current");
  }
  return on.textContent ?? "";
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["fr-FR"],
    configurable: true,
  });
  vi.stubGlobal("fetch", vi.fn(personFetch()));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("the routed subtree's key", () => {
  it("keeps the record's chrome while a tab change swaps the panel", async () => {
    window.location.hash = "#/contacts/p-1/overview";
    renderApp();
    await waitFor(() => expect(currentTab()).toBe("Overview"));
    const chromeBefore = recordHead();
    const stripBefore = tabStrip();

    window.location.hash = "#/contacts/p-1/deals";
    await waitFor(() => expect(currentTab()).toBe("Deals"));

    // The same NODES, not merely equal markup: a remount would have replaced
    // both, and every block between them would have re-animated with them.
    expect(recordHead()).toBe(chromeBefore);
    expect(tabStrip()).toBe(stripBefore);
  });

  it("still throws the page away when the record itself changes", async () => {
    window.location.hash = "#/contacts/p-1/overview";
    renderApp();
    await waitFor(() => expect(currentTab()).toBe("Overview"));
    const chromeBefore = recordHead();

    // The control for the case above, and the reason the key exists at all: one
    // person's screen must never be reconciled into another's, or the state it
    // holds — an open drawer, a half-typed note — arrives on the wrong record.
    window.location.hash = "#/contacts/p-2/overview";
    await waitFor(() => expect(recordHead()).not.toBe(chromeBefore));
  });
});
