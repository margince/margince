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
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { LeadScreen } from "./leads";

// The Deals tab's two panels: the deal `qualified_deal_id` names, and the
// project `project_id` names. Driven through `LeadScreen`, the real wiring,
// because the project verb saves through the page's own shared writer.

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

// Opens the Deals tab and hands back its body, where the deal and project
// verbs live.
async function openDealsTab(
  user: ReturnType<typeof userEvent.setup>,
): Promise<HTMLElement> {
  await user.click(
    await screen.findByRole("button", { name: en["tab.deals"] }),
  );
  await screen.findByRole("heading", { name: en["tab.deals"] });
  return document.querySelector(".record-stack") as HTMLElement;
}

describe("LeadScreen: the Deals tab's deal and project verbs", () => {
  beforeEach(() => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  });

  it("the Attach verb opens the picker, and picking patches project_id", async () => {
    const user = userEvent.setup();
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
    const tab = await openDealsTab(user);
    await user.click(
      within(tab).getByRole("button", {
        name: en["lead.rail.project.attach"],
      }),
    );
    const dialog = screen.getByRole("dialog");
    await user.type(within(dialog).getByRole("combobox"), "Beacon");
    await user.click(
      await within(dialog).findByRole("option", { name: /Beacon rollout/ }),
    );

    await waitFor(() => expect(patched.length).toBe(1));
    expect(patched[0]).toMatchObject({ project_id: "pr-2" });
    // The list closes on the write it made.
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("keeps the picker open over a refused attach, saying why, until Escape hands focus back", async () => {
    const user = userEvent.setup();
    stubLeadScreenFetch(async (url, method) => {
      if (method === "GET" && url.includes("/projects")) {
        return jsonResponse({
          data: [{ id: "pr-2", name: "Beacon rollout", key: "BEA" }],
        });
      }
      if (method === "PATCH" && url.includes("/leads/l-1")) {
        return jsonResponse(
          { title: "Forbidden", detail: "This lead is locked." },
          403,
        );
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
    const tab = await openDealsTab(user);
    const attach = within(tab).getByRole("button", {
      name: en["lead.rail.project.attach"],
    });
    await user.click(attach);
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByRole("combobox"), "Beacon");
    await user.click(
      await dialog.findByRole("option", { name: /Beacon rollout/ }),
    );

    expect(await dialog.findByText("This lead is locked.")).toBeTruthy();
    expect(screen.getByRole("dialog")).toBeTruthy();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(attach);
  });

  it("the deal panel's Qualify verb opens the same dialog the header's own does", async () => {
    const user = userEvent.setup();
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
    const tab = await openDealsTab(user);
    await user.click(
      within(tab).getByRole("button", { name: en["lead.promote"] }),
    );
    // The dialog's own reason line, derived from the lead rather than any
    // response the promote-preview or pipelines fetches happen to carry:
    // seeing it proves the SAME QualifyDialog opened, not a copy of it.
    expect(
      await screen.findByText(en["lead.qualify.reasonHuman"]),
    ).toBeTruthy();
  });

  it("carries no Qualify verb once the lead already named a deal", async () => {
    const user = userEvent.setup();
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

    const tab = await openDealsTab(user);
    const link = await within(tab).findByRole("link", {
      name: "Nordwind Renewal",
    });
    expect(link.getAttribute("href")).toBe("/#/deals/d-9");
    expect(
      within(tab).queryByRole("button", { name: en["lead.promote"] }),
    ).toBeNull();
  });

  it("draws no Qualify verb but refuses Attach with the page's one reason, on a terminal lead", async () => {
    const user = userEvent.setup();
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
    const tab = await openDealsTab(user);
    // Gone, the same terms the header's own Qualify is gone on: a closed
    // lead is not a qualifiable one.
    expect(
      within(tab).queryByRole("button", { name: en["lead.promote"] }),
    ).toBeNull();
    // Attach stays, refused: the Details row that writes the same field is
    // refused the same way rather than hidden.
    const attach = await within(tab).findByRole("button", {
      name: en["lead.rail.project.attach"],
    });
    expect(attach.hasAttribute("disabled")).toBe(true);
    expect(attach.getAttribute("aria-describedby")).toBeTruthy();
  });
});

describe("LeadScreen: Deals tab", () => {
  beforeEach(() => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  });

  it("opens onto the lead's deal, under a Deals panel, and the project it names", async () => {
    const user = userEvent.setup();
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
      name: en["tab.deals"],
    });
    await user.click(tab);
    expect(
      await screen.findByRole("heading", { name: en["tab.deals"] }),
    ).toBeTruthy();

    expect(
      (await screen.findAllByRole("link", { name: "Nordwind Renewal" })).length,
    ).toBeGreaterThan(0);
    expect(
      (await screen.findAllByRole("link", { name: "Atlas rollout" })).length,
    ).toBeGreaterThan(0);
    // Both cards are already claimed, a deal earned and a project attached,
    // so the project panel offers Change rather than Attach.
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
