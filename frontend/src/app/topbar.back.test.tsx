// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate, useRoute } from "../lib/router";
import { sessionOnlyFetch } from "../testing/appharness";
import { ignoreSearch, newClient, renderWith } from "./testing/shellharness";
import { TopBar } from "./topbar";

// The way back from a record. The installed app runs standalone, with no
// browser Back, so the bar's own control and the trail's parent are the only
// ways out, and both must land on the list as the reader left it.
//
// The bar is mounted over the real address store, so every move here is a
// real hash change, and Back is the real `history.back()`.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The bar as the shell mounts it: it re-renders with whatever the address says.
function RoutedTopBar() {
  const route = useRoute();
  return <TopBar route={route} collapsed={false} onOpenSearch={ignoreSearch} />;
}

// Arrives at `hash` the way a cold page load does. The bar mounts only after
// the arrival's `hashchange` has been delivered, so the app sees no move of its
// own; that event also drains any change an earlier hook left queued.
async function mountAt(hash: string) {
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  const arrived = new Promise((resolve) =>
    window.addEventListener("hashchange", resolve, { once: true }),
  );
  window.location.hash = hash;
  await arrived;
  const client = newClient();
  client.setQueryData(["contact", "ref", "p-anna"], "Anna Weber");
  return renderWith(client, <RoutedTopBar />);
}

const trail = () => screen.getByRole("navigation", { name: "Breadcrumb" });

describe("Top bar way back from a record", () => {
  // First in the file on purpose: `backStaysInApp` is per page load, and this
  // case is a record opened before the app has moved anywhere.
  it("goes to the plain list when the record was opened cold", async () => {
    const user = userEvent.setup();
    const back = vi.spyOn(globalThis.history, "back");
    await mountAt("#/contacts/p-anna");

    await user.click(screen.getByRole("button", { name: "Back" }));

    await waitFor(() => expect(window.location.hash).toBe("#/contacts"));
    expect(back).not.toHaveBeenCalled();
  });

  it("returns to the filtered list a record was opened from", async () => {
    const user = userEvent.setup();
    await mountAt("#/contacts?city=Berlin&sort=name");
    // A list is the page itself, with nothing above it to go back to.
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();

    navigate({ screen: "contacts", id: "p-anna" });
    const parent = await within(trail()).findByRole("link", {
      name: "Contacts",
    });
    expect(parent.getAttribute("href")).toBe(
      "#/contacts?city=Berlin&sort=name",
    );

    const back = vi.spyOn(globalThis.history, "back");
    await user.click(screen.getByRole("button", { name: "Back" }));

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts?city=Berlin&sort=name"),
    );
    // History, not a fresh push: the list's entry, scroll offset and all.
    expect(back).toHaveBeenCalledTimes(1);
  });

  // The Ask dialog stands over a list rather than being part of it, so a way
  // back must not reopen a question the reader had already closed.
  it("keeps the filter and drops the Ask dialog from the trail's parent", async () => {
    await mountAt("#/contacts?ask=1&askq=Who&city=Hamburg");

    navigate({ screen: "contacts", id: "p-anna" });
    const parent = await within(trail()).findByRole("link", {
      name: "Contacts",
    });
    expect(parent.getAttribute("href")).toBe("#/contacts?city=Hamburg");
  });
});
