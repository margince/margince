/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { appHref } from "./recordaddress";
import { useRoute } from "./router";

const DEAL = "01a00000-0000-7000-8000-000000000001";
const CONTACT = "01a00000-0000-7000-8000-000000000002";

afterEach(() => {
  cleanup();
  history.replaceState(null, "", "/");
});

it("makes a legacy record address visible to fetchers without losing its view or history state", () => {
  const state = { scrollEntry: "kept" };
  history.replaceState(state, "", `/#/deals/${DEAL}?from=worklist`);
  const view = renderHook(useRoute);
  expect(view.result.current).toMatchObject({ screen: "deals", id: DEAL });
  expect(location.pathname).toBe(`/r/deals/${DEAL}`);
  expect(location.hash).toBe(`#/deals/${DEAL}?from=worklist`);
  expect(history.state).toMatchObject(state);
});

it("keeps the public card identity aligned when a hash link opens another record or leaves it", () => {
  history.replaceState(null, "", `/#/deals/${DEAL}`);
  const view = renderHook(useRoute);
  act(() => {
    location.hash = `#/contacts/${CONTACT}`;
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  expect(view.result.current).toMatchObject({
    screen: "contacts",
    id: CONTACT,
  });
  expect(location.pathname).toBe(`/r/contacts/${CONTACT}`);
  act(() => {
    location.hash = "#/home";
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  expect(view.result.current.screen).toBe("home");
  expect(location.pathname).toBe("/");
});

it("publishes no record path for a list, malformed identifier or credential screen", () => {
  for (const hash of [
    "#/deals",
    "#/companies/not-an-id",
    "#/reset-password?token=secret",
  ]) {
    history.replaceState(null, "", `/${hash}`);
    const view = renderHook(useRoute);
    expect(location.pathname).toBe("/");
    view.unmount();
  }
});

it("gives native record links the same public destination without changing other routes", () => {
  expect(appHref(`#/deals/${DEAL}`)).toBe(`/r/deals/${DEAL}#/deals/${DEAL}`);
  expect(appHref("#/settings/account")).toBe("/#/settings/account");
});

it("restores a bare record address before routing without overwriting another hash", () => {
  history.replaceState(null, "", `/r/deals/${DEAL}`);
  const view = renderHook(useRoute);
  expect(view.result.current).toMatchObject({ screen: "deals", id: DEAL });
  view.unmount();
  history.replaceState(null, "", `/r/deals/${DEAL}#/home`);
  const home = renderHook(useRoute);
  expect(home.result.current.screen).toBe("home");
});
