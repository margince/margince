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
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AutomationsAdmin } from "./automations";
import { ConfiguredAutomations } from "./automations.instances";
import { paramFields } from "./automations.params";

// B-EP09.15 acceptance: the editor is catalog-driven end to end — the
// anti-DSL guard (no free-form rule body, no user-defined trigger; form
// fields derive only from params_schema + name), the autonomy tier badge
// on every row, create-arrives-paused with the deliberate enable step
// (PATCH + If-Match), and authorship-blind rendering (the row is a pure
// function of the Automation wire schema).

beforeEach(() => {
  // the surface sits behind the auth gate in the app; the useMe probe needs a
  // resolved workspace before it will ask /v1/me
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];

const dueInDaysSchema = (fallback: number) => ({
  type: "object",
  properties: {
    due_in_days: {
      type: "integer",
      minimum: 1,
      maximum: 30,
      default: fallback,
    },
  },
  required: ["due_in_days"],
});

const catalog: CatalogEntry[] = [
  {
    key: "stalled_deal_nudge",
    name: "Stalled-deal nudge",
    description: "Stages a follow-up, e.g. a call. The dialog says which.",
    trigger: "deal.stalled",
    action: "send_email",
    tier: "confirmation_required",
    params_schema: dueInDaysSchema(3),
  },
  {
    key: "task_on_stage_entry",
    name: "Task on stage entry",
    trigger: "deal.stage_changed",
    action: "create_task",
    tier: "auto_execute",
    params_schema: dueInDaysSchema(7),
  },
];

type Recorded = {
  method: string;
  url: string;
  body: unknown;
  ifMatch: string | null;
};

// The surface asks four separate questions of the automation object, so the
// default fixture answers all four. `read` is not decoration: AutomationStore's
// List gates on it, so the instances query is disabled without it and an
// operator fixture missing it would prove the list renders for a seat the
// server would refuse.
const AUTOMATION_OPERATOR: GrantSpec = {
  automation: ["create", "read", "update", "delete"],
};

// The verbs a row folds behind its overflow control. Opening it is a step the
// reader takes deliberately, so a test asserting on Edit / Runs / Preview /
// Delete takes it too.
async function openRowMenu(name = "Nudge stalled fleet deals") {
  await userEvent.click(
    screen.getByRole("button", { name: `Actions for ${name}` }),
  );
}

// Authoring is a name plus every parameter the schema declares, submitted
// together, so it lives in a dialog behind the library's verb. A test whose
// subject is one of those fields opens the dialog first and scopes its queries
// to it — the card behind it still carries the library the verb came from.
async function openCreateDialog(index = 0): Promise<HTMLElement> {
  await userEvent.click(
    screen.getAllByRole("button", { name: /^Use template/ })[index],
  );
  return screen.getByRole("dialog");
}

// The same, for a configured automation's own definition.
async function openRowEditor(name = "Nudge stalled fleet deals") {
  await openRowMenu(name);
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  return screen.getByRole("dialog");
}

// React mints an id per render tree, and a second tree in the same document
// gets different ones. They say nothing about the row's CONTENT, which is what
// the authorship-blindness check is about, so the comparison drops them.
function withoutGeneratedIds(html: string): string {
  return html.replaceAll(/_r_[0-9a-z]+_/g, "_rid_");
}

function automationsBackend(
  automations: Automation[],
  calls: Recorded[],
  allow: GrantSpec = AUTOMATION_OPERATOR,
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (url.includes("/automations/catalog")) {
      return jsonResponse({ data: catalog });
    }
    if (url.includes("/automations") && method === "POST") {
      const body: unknown = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      calls.push({ method, url, body, ifMatch: null });
      const created: Automation = {
        id: `au-${automations.length + 1}`,
        ...(body as {
          key: string;
          name: string;
          params: Automation["params"];
        }),
        status: "paused",
        version: 1,
        created_at: "2026-07-05T08:00:00Z",
      };
      automations.push(created);
      return jsonResponse(created, 201);
    }
    if (/\/automations\/au-\d+$/.test(url) && method === "PATCH") {
      const body: unknown = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      calls.push({
        method,
        url,
        body,
        ifMatch: request?.headers.get("If-Match") ?? null,
      });
      return jsonResponse({ ...automations[0], status: "enabled" });
    }
    if (url.includes("/automations")) {
      return jsonResponse({ data: automations, page: { next_cursor: null } });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

const instance = (over: Partial<Automation>): Automation => ({
  id: "au-1",
  key: "stalled_deal_nudge",
  name: "Nudge stalled fleet deals",
  status: "paused",
  params: { due_in_days: 3 },
  version: 3,
  created_at: "2026-07-01T08:00:00Z",
  ...over,
});

describe("AutomationsAdmin (B-EP09.15)", () => {
  // It renders INSIDE the Settings → AI page, which already owns the `.wrap`
  // reading column and the h1 naming the tab. A nested column double-pads the
  // page and a second h1 gives the document two page titles, so this surface
  // contributes neither.
  it("renders as a settings section: no reading column of its own, no page heading", async () => {
    vi.stubGlobal("fetch", automationsBackend([], []));
    const { container } = render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Stalled-deal nudge")).toBeTruthy(),
    );
    expect(container.querySelector(".wrap")).toBeNull();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
    expect(
      screen.queryByRole("heading", { level: 2, name: "Automations" }),
    ).toBeNull();
  });

  it("anti-DSL guard: form fields derive only from params_schema + name — no rule body, no trigger input", async () => {
    vi.stubGlobal("fetch", automationsBackend([], []));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Stalled-deal nudge")).toBeTruthy(),
    );
    const dialog = await openCreateDialog();
    // exactly the name field + the one schema-derived integer parameter
    expect(within(dialog).getAllByRole("textbox")).toHaveLength(1);
    expect(within(dialog).getAllByRole("spinbutton")).toHaveLength(1);
    expect(
      within(dialog).getByRole("spinbutton", { name: "due_in_days" }),
    ).toBeTruthy();
    // No free-form rule body, no user-defined trigger, anywhere on the page —
    // the DOCUMENT, not the render container: the dialog is portalled to the
    // body, so a container-scoped count would pass by looking somewhere the
    // form is not.
    expect(document.querySelectorAll("textarea")).toHaveLength(0);
    expect(screen.queryByRole("textbox", { name: /trigger/i })).toBeNull();
    // the schema bounds reach the input verbatim
    const param = screen.getByRole("spinbutton", { name: "due_in_days" });
    expect(param.getAttribute("min")).toBe("1");
    expect(param.getAttribute("max")).toBe("30");
  });

  it("paramFields reads only typed schema properties", () => {
    expect(paramFields(dueInDaysSchema(3))).toEqual([
      { key: "due_in_days", kind: "integer", min: 1, max: 30, initial: "3" },
    ]);
    expect(paramFields({})).toEqual([]);
  });

  it("create posts key+name+params, arrives paused, and enable is the deliberate If-Match PATCH", async () => {
    const automations: Automation[] = [];
    const calls: Recorded[] = [];
    vi.stubGlobal("fetch", automationsBackend(automations, calls));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Stalled-deal nudge")).toBeTruthy(),
    );
    await openCreateDialog();
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toMatchObject({
      key: "stalled_deal_nudge",
      name: "Stalled-deal nudge",
      params: { due_in_days: 3 },
    });
    // the honest post-create state: paused until the user enables it
    await waitFor(() =>
      expect(
        screen.getByText("Created paused. Nothing runs until it is enabled."),
      ).toBeTruthy(),
    );
    const row = await screen.findByTestId("automation-au-1");
    // The state is the switch's own, announced rather than restated beside it
    // in a second vocabulary: a created automation arrives off.
    expect(
      within(row)
        .getByRole("switch", { name: /is enabled$/ })
        .getAttribute("aria-checked"),
    ).toBe("false");
    await userEvent.click(
      screen.getByRole("switch", { name: "Stalled-deal nudge is enabled" }),
    );
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1].body).toMatchObject({ status: "enabled" });
    expect(calls[1].ifMatch).toBe("1");
  });

  // Running by itself is the normal mode and says nothing; only a rule whose
  // actions wait for approval carries a badge, beside its name.
  it("marks only a rule that needs approval, beside its name", async () => {
    const automations = [
      instance({
        id: "au-1",
        key: "stalled_deal_nudge",
        name: "Confirmation-required one",
      }),
      instance({
        id: "au-2",
        key: "task_on_stage_entry",
        name: "Auto-execute one",
        params: { due_in_days: 7 },
      }),
    ];
    vi.stubGlobal("fetch", automationsBackend(automations, []));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Confirmation-required one")).toBeTruthy(),
    );
    const confirmationRequired = screen.getByTestId("automation-au-1");
    const autoExecute = screen.getByTestId("automation-au-2");
    const badge = within(confirmationRequired).getByText("Needs approval");
    expect(
      within(confirmationRequired)
        .getByText("Confirmation-required one")
        .parentElement?.contains(badge),
    ).toBe(true);
    expect(within(autoExecute).queryByText("Needs approval")).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Mode" })).toBeNull();
  });

  it("renders an instance from the wire schema alone — authorship cannot change the row", async () => {
    // origin is not on the wire: two instances with identical Automation
    // fields (one imagined agent-authored, one catalog-authored) MUST
    // produce identical markup.
    vi.stubGlobal("fetch", automationsBackend([], []));
    const fields = instance({});
    const first = render(
      <ConfiguredAutomations
        automations={[{ ...fields }]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit
        canDelete
      />,
    );
    const firstHtml = withoutGeneratedIds(first.container.innerHTML);
    cleanup();
    const second = render(
      <ConfiguredAutomations
        automations={[{ ...fields }]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit
        canDelete
      />,
    );
    expect(withoutGeneratedIds(second.container.innerHTML)).toBe(firstHtml);
  });

  it("a role without the automation config grant gets the honest read-only editor", async () => {
    // manager/rep hold read-only automation grants: the
    // surface still shows catalog + instances, but no affordance that could
    // only 403 — and it says WHY instead of silently thinning out.
    const automations = [instance({})];
    vi.stubGlobal(
      "fetch",
      automationsBackend(automations, [], { automation: ["read"] }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    expect(
      screen.getByText(
        "Read-only: you do not have permission to change automations.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Use template/ })).toBeNull();
    // No switch to flip, and the badge in its place so the state is still a
    // read this row answers.
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.getByText("Paused")).toBeTruthy();
    await openRowMenu();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
  });

  // One grant at a time. A fixture holding create+update+delete together
  // cannot tell a correct binding from a transposed one — swapping update and
  // delete in the screen passes such a suite outright. These pin each verb to
  // the control it actually governs.
  it("shows only the affordances the specific grant covers", async () => {
    const automations = [instance({})];
    vi.stubGlobal(
      "fetch",
      automationsBackend(automations, [], { automation: ["read", "update"] }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    // update -> the setting writes when you flip it, so it is a switch
    expect(
      screen.getAllByRole("switch", {
        name: "Nudge stalled fleet deals is enabled",
      }).length,
    ).toBe(1);
    await openRowMenu();
    expect(screen.getByRole("button", { name: "Edit" })).toBeTruthy();
    // no delete grant -> the destructive control is withheld
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
    // no create grant -> the catalog cannot be instantiated
    expect(screen.queryByRole("button", { name: /^Use template/ })).toBeNull();
  });

  it("offers deletion only with the delete grant, and nothing else with it", async () => {
    const automations = [instance({})];
    vi.stubGlobal(
      "fetch",
      automationsBackend(automations, [], { automation: ["read", "delete"] }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    await openRowMenu();
    expect(screen.getAllByRole("button", { name: "Delete" }).length).toBe(1);
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Use template/ })).toBeNull();
  });

  it("deleting asks first — the confirm is what writes, not the menu item", async () => {
    const automations = [instance({})];
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      automationsBackend(automations, calls, {
        automation: ["read", "delete"],
      }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    await openRowMenu();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    // The dialog names the automation and says what pausing would do instead;
    // nothing has been written yet.
    expect(
      screen.getByRole("heading", { name: "Delete this automation?" }),
    ).toBeTruthy();
    expect(calls.filter((call) => call.method === "DELETE")).toHaveLength(0);
  });

  // The runs and preview inspectors are READS (automations_runs.go gates on
  // automation:read; Preview resolves through Get). A read-only principal must
  // still reach them.
  it("offers the runs and preview inspectors on the read grant alone", async () => {
    const automations = [instance({})];
    vi.stubGlobal(
      "fetch",
      automationsBackend(automations, [], { automation: ["read"] }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    await openRowMenu();
    expect(screen.getAllByRole("button", { name: "Runs" }).length).toBe(1);
    expect(screen.getAllByRole("button", { name: "Preview" }).length).toBe(1);
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
  });

  // Two lists, two panes, each named for what it holds; neither repeats the
  // page's own title.
  it("gives the configured rules and the starter library a pane each", async () => {
    vi.stubGlobal("fetch", automationsBackend([instance({})], []));
    render(<AutomationsAdmin />);
    await screen.findByText("Nudge stalled fleet deals");
    expect(
      screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
    ).toEqual(["Configured automations", "Starter library"]);
    for (const name of ["Configured automations", "Starter library"]) {
      expect(screen.getByRole("region", { name })).toBeTruthy();
    }
  });

  // Every template is one row: its name, what it does, and what sets it off in
  // words. The wire's own identifiers never reach the screen.
  it("gives every library entry a row with its trigger in words", async () => {
    vi.stubGlobal("fetch", automationsBackend([], []));
    render(<AutomationsAdmin />);
    const stalled = await screen.findByTestId("template-stalled_deal_nudge");
    expect(screen.getByTestId("template-task_on_stage_entry")).toBeTruthy();
    expect(within(stalled).getByText("Stalled-deal nudge")).toBeTruthy();
    // The row reads the first sentence only, and "e.g." does not end one.
    expect(
      within(stalled).getByText("Stages a follow-up, e.g. a call."),
    ).toBeTruthy();
    expect(stalled).not.toHaveTextContent(/The dialog says|send_email|->/);
    expect(
      within(screen.getByTestId("template-task_on_stage_entry")).getByText(
        "Deal stage changed",
      ),
    ).toBeTruthy();
  });

  it("opens a configured automation's definition in a dialog, not under the row", async () => {
    const automations = [instance({})];
    vi.stubGlobal("fetch", automationsBackend(automations, []));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    // Nothing to edit with until the verb is used: the row is an answer.
    expect(screen.queryByRole("dialog")).toBeNull();
    const dialog = await openRowEditor();
    // The dialog names WHICH automation is open, because it covers the row that
    // would otherwise have said.
    expect(
      within(dialog).getByRole("heading", {
        name: "Nudge stalled fleet deals",
      }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("spinbutton", { name: "due_in_days" }),
    ).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Save" })).toBeTruthy();
  });

  it("reports a refused definition edit inside the dialog, never behind it", async () => {
    const automations = [instance({})];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const url = String(request ? request.url : input);
        const method = request ? request.method : (init?.method ?? "GET");
        if (url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: AUTOMATION_OPERATOR }));
        }
        if (url.includes("/automations/catalog")) {
          return jsonResponse({ data: catalog });
        }
        if (/\/automations\/au-\d+$/.test(url) && method === "PATCH") {
          return jsonResponse(
            {
              title: "Conflict",
              status: 409,
              detail: "somebody else changed this automation",
            },
            409,
          );
        }
        return jsonResponse({
          data: automations,
          page: { next_cursor: null },
        });
      }),
    );
    const { container } = render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    const dialog = await openRowEditor();
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    // Where the reader still is. The card behind the dialog says nothing: a
    // refusal reported there is a refusal reported behind the thing covering
    // it.
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      /somebody else changed this automation/,
    );
    expect(container.querySelector('[role="alert"]')).toBeNull();
  });

  it("the config affordances stay for admin", async () => {
    const automations = [instance({})];
    vi.stubGlobal("fetch", automationsBackend(automations, []));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );
    expect(
      screen.queryByText(
        "Read-only: you do not have permission to change automations.",
      ),
    ).toBeNull();
    await waitFor(() =>
      expect(
        screen.getAllByRole("button", { name: /^Use template/ }).length,
      ).toBeGreaterThan(0),
    );
    expect(screen.getByRole("switch", { name: /is enabled$/ })).toBeTruthy();
    await openRowMenu();
    expect(screen.getByRole("button", { name: "Delete" })).toBeTruthy();
  });
});

// The two ops behind these toggles are human-only and gated on the same
// automation:update grant as pause and edit; the panels mount lazily and
// independently (opening one never closes the other).
describe("ConfiguredAutomations — Runs/Preview toggles", () => {
  const previewTitle = "Dry-run impact";
  // A benign stub for the lazily-mounted panels' first fetch: these tests are
  // about mount and independence, so runs answer empty and preview zero.
  function panelBackend() {
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.includes("/preview") && method === "POST") {
        return jsonResponse({
          matches_now: 0,
          would_have_fired: 0,
          window_days: 30,
        });
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    });
  }

  it("shows the Runs/Preview toggles on the READ grant, not the write one", async () => {
    vi.stubGlobal("fetch", panelBackend());
    const view = render(
      <ConfiguredAutomations
        automations={[instance({})]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit
        canDelete
      />,
    );
    await openRowMenu();
    expect(screen.getByRole("button", { name: "Runs" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Preview" })).toBeTruthy();
    view.unmount();

    // Read but no write: the inspectors stay, because the server gates them on
    // automation:read. Hiding them behind the write grant — as the old role
    // proxy did — withheld a surface a reader is entitled to.
    const readOnly = render(
      <ConfiguredAutomations
        automations={[instance({})]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit={false}
        canDelete={false}
      />,
    );
    await openRowMenu();
    expect(screen.getByRole("button", { name: "Runs" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Preview" })).toBeTruthy();
    readOnly.unmount();

    // No read grant: gone. The menu is still there — Edit and Delete are the
    // caller's other two grants — so the assertion is about the items, not
    // about the control that holds them.
    render(
      <ConfiguredAutomations
        automations={[instance({})]}
        entryFor={() => catalog[0]}
        canViewRuns={false}
        canEdit
        canDelete
      />,
    );
    await openRowMenu();
    expect(screen.queryByRole("button", { name: "Runs" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Preview" })).toBeNull();
  });

  // Alone, the run history is the row's whole detail and needs no heading;
  // the row's chevron already names it.
  it("mounts the run history on click without mounting preview", async () => {
    vi.stubGlobal("fetch", panelBackend());
    render(
      <ConfiguredAutomations
        automations={[instance({})]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit
        canDelete
      />,
    );
    expect(screen.queryByText("This automation has not fired yet.")).toBeNull();
    await openRowMenu();
    await userEvent.click(screen.getByRole("button", { name: "Runs" }));
    expect(
      await screen.findByText("This automation has not fired yet."),
    ).toBeTruthy();
    expect(screen.queryByRole("heading")).toBeNull();
    expect(screen.queryByText(/estimate/i)).toBeNull();
  });

  // Side by side, each inspector takes a heading so a reader tells them apart.
  it("keeps both panels open independently", async () => {
    vi.stubGlobal("fetch", panelBackend());
    render(
      <ConfiguredAutomations
        automations={[instance({})]}
        entryFor={() => catalog[0]}
        canViewRuns
        canEdit
        canDelete
      />,
    );
    await openRowMenu();
    await userEvent.click(screen.getByRole("button", { name: "Runs" }));
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByRole("heading", { name: "Run history" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: previewTitle })).toBeTruthy();
  });
});
