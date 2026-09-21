/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { LeadDealSection } from "./leadraillinks";
import { LeadScreen } from "./leads";

// The lead's own two rail cards: the deal `qualified_deal_id` names, and the
// project `project_id` names. Each has one state a lead of neither field
// reads, and one it reads once qualified or once a project is set on it.
// `LeadDealSection` is exercised directly, since it takes no writer, only
// the `onQualify` callback the page hands every door to the same dialog. The
// project slice takes the lead's own shared writer (`useLeadPatch`, private
// to leads.tsx) and is exercised through `LeadScreen`, the real wiring,
// rather than a hand-built stand-in for it.

type Lead = components["schemas"]["Lead"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  captured_by: "human:u-1",
  source: "manual",
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

describe("LeadDealSection", () => {
  it("names the deal the lead was qualified into, and offers no further verb", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/deals/d-9")) {
          return jsonResponse({ id: "d-9", name: "Nordwind Renewal" });
        }
        return jsonResponse({}, 404);
      }),
    );
    render(
      <LeadDealSection
        lead={{ ...lead, qualified_deal_id: "d-9" }}
        onQualify={vi.fn()}
      />,
    );

    const link = await screen.findByRole("link", {
      name: "Nordwind Renewal",
    });
    expect(link.getAttribute("href")).toBe("#/deals/d-9");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("offers Qualify, the header's own door, while there is still a deal to earn", async () => {
    const onQualify = vi.fn();
    render(
      <LeadDealSection
        lead={{ ...lead, qualified_deal_id: null }}
        onQualify={onQualify}
      />,
    );

    await screen.findByText(en["lead.rail.deal.empty"]);
    await userEvent.click(
      screen.getByRole("button", { name: en["lead.promote"] }),
    );
    expect(onQualify).toHaveBeenCalled();
  });

  it("refuses Qualify with the page's one reason, while a live lead stays this caller's to write but not to qualify", async () => {
    // The reason band is for a verb that STAYS: a caller who may not write
    // this lead sees Qualify, disabled, with the page's one sentence, the
    // same shape the header's own Qualify takes on the same lead.
    render(
      <>
        <p id="not-yours">You cannot change this lead.</p>
        <LeadDealSection
          lead={{ ...lead, qualified_deal_id: null }}
          onQualify={vi.fn()}
          reasonId="not-yours"
        />
      </>,
    );

    const button = await screen.findByRole("button", {
      name: en["lead.promote"],
    });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(button.getAttribute("aria-describedby")).toBe("not-yours");
  });

  it("draws no Qualify verb once the lead is closed, the same terms the header draws it on", async () => {
    render(
      <LeadDealSection
        lead={{
          ...lead,
          qualified_deal_id: null,
          archived_at: "2026-07-13T00:00:00Z",
        }}
        onQualify={vi.fn()}
      />,
    );

    await screen.findByText(en["lead.rail.deal.empty"]);
    expect(screen.queryByRole("button")).toBeNull();
  });
});

// Base transport every LeadScreen render needs before it will show the
// record at all: the session, the mail connectors the shell reads for every
// page, the anchor context, and the two administered catalogs the Details
// card's own rows read off. Given here rather than duplicated per test, the
// same reason `stubFetch` in leads.test.tsx exists.
const LEAD_GRANTS = {
  lead: ["read", "create", "update", "delete"],
  activity: ["read", "create"],
} as const;

function stubLeadScreenFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response> | Response,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = request.url;
      if (url.endsWith("/v1/me")) {
        return jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow: LEAD_GRANTS }).authorization,
        });
      }
      if (url.endsWith("/v1/connectors")) {
        return jsonResponse({ data: [] });
      }
      if (new URL(url).pathname.endsWith("/context")) {
        return jsonResponse({
          anchor: { type: "lead", id: "l-1" },
          sections: [],
        });
      }
      if (url.endsWith("/v1/lead-sources")) {
        return jsonResponse({ data: [] });
      }
      if (url.endsWith("/v1/leads/settings")) {
        return jsonResponse({
          first_response_enabled: false,
          first_response_target_minutes: 240,
        });
      }
      return await responder(url, request.method, request);
    }),
  );
}

describe("LeadScreen: the rail's own deal and project verbs", () => {
  beforeEach(() => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  });

  it("the rail's Attach verb opens the picker, and picking patches project_id", async () => {
    const patched: unknown[] = [];
    stubLeadScreenFetch(async (url, method, request) => {
      if (method === "GET" && url.includes("/projects")) {
        return jsonResponse({
          data: [{ id: "pr-2", name: "Beacon rollout", key: "BEA" }],
        });
      }
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        patched.push(await request.json());
        return jsonResponse({ ...lead, project_id: "pr-2" });
      }
      if (url.includes("/leads/l-1")) {
        return jsonResponse(lead);
      }
      return jsonResponse({ data: [] });
    });

    render(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const rail = document.querySelector(".co-rail") as HTMLElement;
    await userEvent.click(
      within(rail).getByRole("button", {
        name: en["lead.rail.project.attach"],
      }),
    );
    const dialog = screen.getByRole("dialog");
    await userEvent.type(within(dialog).getByRole("searchbox"), "Beacon");
    await userEvent.click(
      await within(dialog).findByRole("button", { name: /Beacon rollout/ }),
    );

    await waitFor(() => expect(patched.length).toBe(1));
    expect(patched[0]).toMatchObject({ project_id: "pr-2" });
    // The dialog closes on the write it made, the same shape the account's
    // own project attach follows.
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("the rail's Qualify verb opens the same dialog the header's own does", async () => {
    stubLeadScreenFetch(async (url) => {
      if (url.includes("/leads/l-1")) {
        return jsonResponse(lead);
      }
      return jsonResponse({ data: [] });
    });

    render(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const rail = document.querySelector(".co-rail") as HTMLElement;
    await userEvent.click(
      within(rail).getByRole("button", { name: en["lead.promote"] }),
    );
    // The dialog's own reason line, derived from the lead rather than any
    // response the promote-preview or pipelines fetches happen to carry:
    // seeing it proves the SAME QualifyDialog opened, not a copy of it.
    expect(
      await screen.findByText(en["lead.qualify.reasonHuman"]),
    ).toBeTruthy();
  });

  it("carries no Qualify verb once the lead already named a deal", async () => {
    stubLeadScreenFetch(async (url) => {
      if (url.includes("/deals/d-9")) {
        return jsonResponse({ id: "d-9", name: "Nordwind Renewal" });
      }
      if (url.includes("/leads/l-1")) {
        return jsonResponse({ ...lead, qualified_deal_id: "d-9" });
      }
      return jsonResponse({ data: [] });
    });

    render(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    const rail = (
      await screen.findByRole("link", {
        name: "Nordwind Renewal",
      })
    ).closest(".co-rail") as HTMLElement;
    expect(
      within(rail).queryByRole("button", { name: en["lead.promote"] }),
    ).toBeNull();
  });

  it("draws no Qualify verb but refuses Attach with the page's one reason, on a terminal lead", async () => {
    stubLeadScreenFetch(async (url) => {
      if (url.includes("/leads/l-1")) {
        return jsonResponse({
          ...lead,
          status: "disqualified",
          archived_at: "2026-07-13T00:00:00Z",
        });
      }
      return jsonResponse({ data: [] });
    });

    render(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    await screen.findByRole("heading", { level: 1, name: "Jonas Petersen" });
    const rail = document.querySelector(".co-rail") as HTMLElement;
    // Gone, the same terms the header's own Qualify is gone on: a closed
    // lead is not a qualifiable one.
    expect(
      within(rail).queryByRole("button", { name: en["lead.promote"] }),
    ).toBeNull();
    // Attach stays, refused: the Details row that writes the same field is
    // refused the same way rather than hidden.
    const attach = await within(rail).findByRole("button", {
      name: en["lead.rail.project.attach"],
    });
    expect(attach.hasAttribute("disabled")).toBe(true);
    expect(attach.getAttribute("aria-describedby")).toBeTruthy();
  });
});

describe("LeadScreen: Deals & projects tab", () => {
  beforeEach(() => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  });

  it("carries the account's own tab label, and opens onto the lead's deal and project", async () => {
    stubLeadScreenFetch((url) => {
      if (url.includes("/deals/d-9")) {
        return jsonResponse({ id: "d-9", name: "Nordwind Renewal" });
      }
      if (url.includes("/projects/pr-1")) {
        return jsonResponse({
          id: "pr-1",
          name: "Atlas rollout",
          phase: "delivering",
        });
      }
      if (url.includes("/leads/l-1")) {
        return jsonResponse({
          ...lead,
          qualified_deal_id: "d-9",
          project_id: "pr-1",
        });
      }
      return jsonResponse({ data: [] });
    });

    render(
      <RecordShell>
        <LeadScreen id="l-1" />
      </RecordShell>,
    );

    const tab = await screen.findByRole("button", {
      name: en["tab.dealsProjects"],
    });
    await userEvent.click(tab);

    // The rail carries the same two cards beside the tab, so both the deal
    // and the project name may sit in the document twice: once in the tab
    // body just opened, once in the column beside it.
    expect(
      (await screen.findAllByRole("link", { name: "Nordwind Renewal" })).length,
    ).toBeGreaterThan(0);
    expect(
      (await screen.findAllByRole("link", { name: "Atlas rollout" })).length,
    ).toBeGreaterThan(0);
    // Both cards are already claimed, a deal earned and a project attached,
    // so the tab's own project panel offers Change rather than Attach, the
    // same word the rail beside it carries.
    const tabBody = document.querySelector(".record-stack") as HTMLElement;
    expect(
      within(tabBody).getByRole("button", {
        name: en["lead.rail.project.change"],
      }),
    ).toBeTruthy();
    expect(
      within(tabBody).queryByRole("button", { name: en["lead.promote"] }),
    ).toBeNull();
  });
});
