// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SorModeChip } from "./sormodechip";

function mount(
  mode: "native" | "overlay" | undefined,
  // Which seat is looking. The mode is reported to every seat; the LINK to the
  // Integrations entry belongs to whoever may CHANGE the installation's wiring,
  // so what this principal holds is part of what the component answers rather
  // than harness detail.
  //
  // Grants, not a role name: the chip asks the settings catalog whether this
  // reader opens the Integrations page. A fixture naming only a role would
  // answer a question the component no longer asks — and an edited admin role
  // really can hold nothing, which is the case a role-name gate would get wrong.
  roles: string[] = ["admin"],
  connects = true,
) {
  const fetchMock = vi.fn(async () => {
    const systemOfRecord = mode ? { system_of_record: { mode } } : {};
    return new Response(
      JSON.stringify({
        user: { id: "u1", email: "a@example.test", display_name: "A" },
        roles,
        teams: [],
        authorization: {
          objects: {
            // The read is always held, because every seeded role holds it —
            // whether capture is working shows up on the records they open.
            // `connects` moves only the write, which is the verb the page asks.
            overlay_connection: {
              read: true,
              create: connects,
              update: connects,
              delete: connects,
            },
          },
        },
        ...systemOfRecord,
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  });
  vi.stubGlobal("fetch", fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SorModeChip />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { fetchMock };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("renders nothing in native mode", async () => {
  const { fetchMock } = mount("native");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole("link")).toBeNull();
});

it("renders nothing when /me omits the field (pre-overlay server)", async () => {
  const { fetchMock } = mount(undefined);
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole("link")).toBeNull();
});

it("links to Settings → Integrations and explains the mode in its label", async () => {
  mount("overlay");
  const link = await screen.findByRole("link");
  // The chip is the one affordance that tells any seat where the connection
  // is managed, so the target is part of its contract: it must be the entry
  // that actually holds the overlay cards, which is the installation-wide one
  // rather than the reader's own connections.
  expect(link.getAttribute("href")).toBe("#/settings/integrations");
  // The chip text itself is too small to carry the explanation — it rides
  // title/aria-label instead. Both must actually name the mode, not just
  // exist, or a screen reader / hover user gets no more than sighted users
  // scanning the two-word chip.
  const explanation = link.getAttribute("aria-label");
  expect(explanation).toBe(link.getAttribute("title"));
  expect(explanation).toMatch(/hubspot/i);
  // The copy names the destination, so it has to name the same tab the href
  // opens — a chip that says one place and goes to another is worse than
  // silent.
  expect(explanation).toMatch(/Settings → Integrations/);
  expect(explanation?.length ?? 0).toBeGreaterThan(20);
});

it("reports the mode to a seat that cannot open Integrations, without linking", async () => {
  // The mode changes what every screen can do, so hiding it from a rep would
  // leave them reading narrowed lists with nothing saying why. What a rep must
  // not get is the affordance: Integrations opens for whoever may change the
  // installation's wiring, and a link that lands on the access boundary is a
  // chip that lied.
  //
  // A rep really does READ `overlay_connection` — that is why the page moved to
  // the write — so this fixture withholds the write rather than the object.
  mount("overlay", ["rep"], false);
  // Found by its TEXT, because a plain span is not a labelable element: the
  // explanation is visually-hidden text inside the chip rather than an
  // `aria-label` a screen reader is free to ignore there.
  const explanation = await screen.findByText(/settings → integrations/i);
  // No jest-dom in this file: the DOM is read directly, as the cases above do.
  expect(explanation.closest("span")?.textContent).toMatch(/hubspot/i);
  expect(screen.queryByRole("link")).toBeNull();
});
