// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate, type Route, useRoute } from "../lib/router";
import { sessionOnlyFetch } from "../testing/appharness";
import { ignoreSearch, newClient, renderWith } from "../testing/shellharness";
import { TopBar } from "./topbar";

// The way back from a record. The installed app has no browser Back. The
// bar's Back and the trail's parent are the only ways out. Both must land on
// the list as the reader left it.
//
// The bar is mounted over the real address store and the real history. Every
// move here is a real hash change, and every step back is a real one.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// The bar as the shell mounts it: it re-renders with whatever the address says.
function RoutedTopBar() {
  const route = useRoute();
  return <TopBar route={route} collapsed={false} onOpenSearch={ignoreSearch} />;
}

// Resolves once the browser has delivered the next `hashchange`. Listened for
// after the app's own listener, so by then the app has seen the move too.
function nextHashChange(): Promise<unknown> {
  return new Promise((resolve) =>
    window.addEventListener("hashchange", resolve, { once: true }),
  );
}

// Arrives at `hash` the way a cold page load does. The bar mounts only after
// the arrival is delivered, so the app sees no move of its own.
async function mountAt(hash: string) {
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  const arrived = nextHashChange();
  window.location.hash = hash;
  await arrived;
  const client = newClient();
  client.setQueryData(["contact", "ref", "p-anna"], "Anna Weber");
  return renderWith(client, <RoutedTopBar />);
}

async function moveTo(route: Route) {
  const moved = nextHashChange();
  navigate(route);
  await moved;
}

async function pressBack(user: ReturnType<typeof userEvent.setup>) {
  const moved = nextHashChange();
  await user.click(screen.getByRole("button", { name: "Back" }));
  await moved;
}

const trail = () => screen.getByRole("navigation", { name: "Breadcrumb" });
const ANNA: Route = { screen: "contacts", id: "p-anna" };

describe("Top bar way back from a record", () => {
  it("goes to the plain list when the record was opened cold", async () => {
    const user = userEvent.setup();
    const go = vi.spyOn(globalThis.history, "go");
    await mountAt("#/contacts/p-anna");

    await pressBack(user);

    expect(window.location.hash).toBe("#/contacts");
    expect(go).not.toHaveBeenCalled();
  });

  // The browser's own Back, where there is one, returns to the record opened
  // cold. Nothing of ours is behind it, so the bar's Back must not step out of
  // the app. In the installed app that step would do nothing at all.
  it("stays in the app after the reader steps back to a record opened cold", async () => {
    const user = userEvent.setup();
    await mountAt("#/contacts/p-anna");
    await pressBack(user);
    const returned = nextHashChange();
    globalThis.history.back();
    await returned;
    expect(window.location.hash).toBe("#/contacts/p-anna");

    const go = vi.spyOn(globalThis.history, "go");
    await pressBack(user);

    expect(window.location.hash).toBe("#/contacts");
    expect(go).not.toHaveBeenCalled();
  });

  it("returns to the filtered list a record was opened from, past its tabs", async () => {
    const user = userEvent.setup();
    await mountAt("#/contacts?city=Berlin&sort=name");
    // A list is the page itself, with nothing above it to go back to.
    expect(screen.queryByRole("button", { name: "Back" })).toBeNull();

    await moveTo(ANNA);
    expect(
      within(trail())
        .getByRole("link", { name: "Contacts" })
        .getAttribute("href"),
    ).toBe("/#/contacts?city=Berlin&sort=name");
    await moveTo({ ...ANNA, id2: "activity" });
    await moveTo({ ...ANNA, id2: "deals" });

    const go = vi.spyOn(globalThis.history, "go");
    await pressBack(user);

    expect(window.location.hash).toBe("#/contacts?city=Berlin&sort=name");
    // History rather than a fresh push: the list's own entry, scroll and all.
    expect(go).toHaveBeenCalledWith(-3);
  });

  it("returns to the screen before the record when it was not opened from a list", async () => {
    const user = userEvent.setup();
    await mountAt("#/home");
    await moveTo(ANNA);
    await moveTo({ ...ANNA, id2: "activity" });

    await pressBack(user);

    expect(window.location.hash).toBe("#/home");
  });

  // The Ask dialog stands over a list and is not part of it. A way back must
  // not reopen a question the reader had already closed.
  it("keeps the filter and drops the Ask dialog from the trail's parent", async () => {
    await mountAt("#/contacts?ask=1&askq=Who&city=Hamburg");

    await moveTo(ANNA);

    expect(
      within(trail())
        .getByRole("link", { name: "Contacts" })
        .getAttribute("href"),
    ).toBe("/#/contacts?city=Hamburg");
  });
});
