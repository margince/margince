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

const SLUG = "Ab3_x-9QrStUvWxYz0123456789abcd";

it("opens a booking page entered at its /book/ path, with or without a trailing slash", () => {
  for (const path of [`/book/${SLUG}`, `/book/${SLUG}/`]) {
    history.replaceState(null, "", path);
    const view = renderHook(useRoute);
    expect(view.result.current).toEqual({ screen: "book", id: SLUG });
    expect(location.pathname).toBe(`/book/${SLUG}`);
    view.unmount();
  }
});

it("keeps a booking slug's case and reads a percent-encoded path as nginx does", () => {
  history.replaceState(null, "", "/book/%41bC");
  const view = renderHook(useRoute);
  expect(view.result.current).toEqual({ screen: "book", id: "AbC" });
  expect(location.pathname).toBe("/book/AbC");
});

it("gives a legacy /#/book/ link the path a preview fetcher reads", () => {
  history.replaceState(null, "", `/#/book/${SLUG}`);
  const view = renderHook(useRoute);
  expect(view.result.current).toEqual({ screen: "book", id: SLUG });
  expect(location.pathname).toBe(`/book/${SLUG}`);
});

it("never puts a guest token or a host control into the server-visible path", () => {
  for (const id of [
    "manage-private-token",
    "proposal-private-token",
    "meeting-01a00000-0000-7000-8000-000000000001",
    "contact-01a00000-0000-7000-8000-000000000002",
    "preview",
    "contact",
  ]) {
    history.replaceState(null, "", `/#/book/${id}`);
    const view = renderHook(useRoute);
    expect(location.pathname, id).toBe("/");
    view.unmount();
    history.replaceState(null, "", `/book/${id}`);
    const typed = renderHook(useRoute);
    expect(location.pathname, id).toBe("/");
    expect(location.hash, id).toBe("");
    typed.unmount();
  }
});

it("moves to the root path when a booking leads to the guest's private management link", () => {
  history.replaceState(null, "", `/book/${SLUG}`);
  const view = renderHook(useRoute);
  act(() => {
    location.hash = "#/book/manage-private-token";
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  expect(view.result.current).toEqual({
    screen: "book",
    id: "manage-private-token",
  });
  expect(location.pathname).toBe("/");
});
