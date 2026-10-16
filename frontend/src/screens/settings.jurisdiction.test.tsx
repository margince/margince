/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { JurisdictionCard } from "./settings.jurisdiction";

type Jurisdiction = components["schemas"]["InstallationJurisdiction"];

const UNSET: Jurisdiction = {
  selected: null,
  available: [
    {
      code: "de",
      unit_name: "de",
      unit_version: "1.0.0",
      description: "German pack",
      correspondence_floor: {
        name: "commercial_correspondence",
        keep: "P6Y",
        from_year_end: true,
      },
    },
    {
      code: "vn",
      unit_name: "vn",
      unit_version: "1.0.0",
      description: "Vietnamese pack",
    },
  ],
  floor: {
    known: true,
    class: "commercial_correspondence",
    keep: "P6Y",
    from_year_end: true,
    packs: ["de", "vn"],
  },
  deadlines: [
    { kind: "access", rule: "gdpr", period: "P1M" },
    { kind: "rectify", rule: "gdpr", period: "P1M" },
    { kind: "erasure", rule: "gdpr", period: "P1M" },
  ],
  documents: [],
};

type Sent = { method: string; path: string; body: unknown };

function stub(page: Jurisdiction, canChange = true, refuse = false): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "https://t");
      const method = request?.method ?? init?.method ?? "GET";
      const body: unknown =
        method === "GET" ? null : request ? await request.json() : null;
      sent.push({ method, path: url.pathname, body });
      const json = (value: unknown) =>
        new Response(JSON.stringify(value), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      if (url.pathname.endsWith("/me")) {
        return json(
          meFixture({
            roles: ["admin"],
            allow: {
              installation_settings: canChange ? ["read", "update"] : ["read"],
            },
          }),
        );
      }
      if (url.pathname.endsWith("/installation/jurisdiction"))
        return json(page);
      if (refuse && method === "PATCH") {
        return new Response(
          JSON.stringify({
            type: "about:blank",
            title: "Change refused",
            status: 422,
            detail: "Try again later.",
          }),
          {
            status: 422,
            headers: { "Content-Type": "application/problem+json" },
          },
        );
      }
      return json({});
    }),
  );
  return sent;
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <JurisdictionCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the jurisdiction page", () => {
  it("says no jurisdiction is set, with the GDPR baseline it falls back to", async () => {
    stub(UNSET);
    render();
    expect(
      await screen.findByText(en["jurisdiction.selected.none"]),
    ).toBeInTheDocument();
    expect(screen.getAllByText(en["jurisdiction.rule.gdpr"])).toHaveLength(3);
    expect(
      screen.getByText(en["jurisdiction.documents.emptyUnset"]),
    ).toBeInTheDocument();
  });

  it("names what a change weakens before it sends it", async () => {
    const sent = stub(UNSET);
    const user = userEvent.setup();
    render();
    await user.click(
      await screen.findByRole("button", {
        name: en["jurisdiction.change.label"],
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: en["jurisdiction.change.label"],
    });
    expect(
      within(dialog).getByRole("button", {
        name: en["jurisdiction.confirm.confirm"],
      }),
    ).toBeDisabled();
    await pickOption(
      user,
      within(dialog).getByRole("combobox", {
        name: en["jurisdiction.card.title"],
      }),
      "Vietnam",
    );
    expect(within(dialog).getByText(/becomes no floor/)).toBeInTheDocument();
    expect(
      within(dialog).getByText(/keeps Commercial correspondence/),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", {
        name: en["jurisdiction.confirm.confirm"],
      }),
    );
    await waitFor(() =>
      expect(sent.find((s) => s.method === "PATCH")).toEqual({
        method: "PATCH",
        path: "/v1/installation/settings",
        body: { country: "vn" },
      }),
    );
  });

  it("keeps its title through a choice and drops a refusal once the choice changes", async () => {
    stub(UNSET, true, true);
    const user = userEvent.setup();
    render();
    await user.click(
      await screen.findByRole("button", {
        name: en["jurisdiction.change.label"],
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: en["jurisdiction.change.label"],
    });
    const choice = within(dialog).getByRole("combobox", {
      name: en["jurisdiction.card.title"],
    });
    await pickOption(user, choice, "Vietnam");
    expect(dialog).toHaveAccessibleName(en["jurisdiction.change.label"]);
    await user.click(
      within(dialog).getByRole("button", {
        name: en["jurisdiction.confirm.confirm"],
      }),
    );
    expect(await within(dialog).findByText(/Try again later/)).toBeVisible();
    await pickOption(user, choice, "Germany");
    expect(within(dialog).queryByText(/Try again later/)).toBeNull();
  });

  it("shows a carried floor and when it stops binding", async () => {
    stub({
      ...UNSET,
      selected: "vn",
      floor: {
        known: false,
        from_year_end: false,
        packs: ["de", "vn"],
        carried: {
          class: "commercial_correspondence",
          keep: "P6Y",
          from_year_end: true,
          before: "2026-10-02T09:00:00Z",
          binds_until: "2033-01-01T00:00:00Z",
        },
      },
    });
    render();
    expect(
      await screen.findByText(en["jurisdiction.effects.floorNone.title"]),
    ).toBeInTheDocument();
    expect(screen.getByText(/for mail from before/)).toBeInTheDocument();
  });

  it("offers no change to a reader who may not make one", async () => {
    stub(UNSET, false);
    render();
    await screen.findByText(en["jurisdiction.selected.none"]);
    expect(
      screen.queryByRole("button", { name: en["jurisdiction.change.label"] }),
    ).toBeNull();
  });
});
