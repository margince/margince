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
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AutomationsAdmin } from "./automations";

// A REFUSAL BELONGS TO THE OPENING THAT EARNED IT.
//
// Both dialogs on this surface outlive their own close now, so they can animate
// out — and a dialog that outlives its close keeps its mutation too. `seq`
// re-seeds the form on each opening and nothing re-seeds the mutation, so
// without a reset the server's last refusal is already printed under the next
// opening: a sentence about a save the reader has not attempted yet.
//
// Its own file rather than a case in automations.test.tsx, which is at its
// length ceiling.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];

const OPERATOR: GrantSpec = {
  automation: ["create", "read", "update", "delete"],
};

const entry: CatalogEntry = {
  key: "stalled_deal_nudge",
  name: "Stalled-deal nudge",
  trigger: "deal.stalled",
  action: "send_email",
  tier: "confirmation_required",
  params_schema: {
    type: "object",
    properties: {
      due_in_days: { type: "integer", minimum: 1, maximum: 30, default: 3 },
    },
    required: ["due_in_days"],
  },
};

const configured: Automation = {
  id: "au-1",
  key: "stalled_deal_nudge",
  name: "Nudge stalled fleet deals",
  status: "enabled",
  params: { due_in_days: 3 },
  version: 4,
  created_at: "2026-08-13T08:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Everything the surface reads, plus one write that the server refuses. `write`
// names the method the refusal answers, so the create path (POST) and the edit
// path (PATCH) can share one backend.
function refusing(write: "POST" | "PATCH", detail: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: OPERATOR }));
      }
      if (url.includes("/automations/catalog")) {
        return jsonResponse({ data: [entry] });
      }
      if (method === write) {
        return jsonResponse({ title: "Refused", status: 409, detail }, 409);
      }
      return jsonResponse({
        data: [configured],
        page: { next_cursor: null },
      });
    }),
  );
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

// The row's verbs live behind its overflow control. The menu is still open the
// second time round — closing the dialog hands focus back to the control it was
// opened from — so pressing that control again would put the menu AWAY.
async function openRowEditor() {
  if (screen.queryByRole("button", { name: "Edit" }) === null) {
    await userEvent.click(
      screen.getByRole("button", { name: `Actions for ${configured.name}` }),
    );
  }
  await userEvent.click(screen.getByRole("button", { name: "Edit" }));
  return screen.getByRole("dialog");
}

describe("a refused write does not survive the dialog that earned it", () => {
  it("leaves the create dialog's next opening clean", async () => {
    refusing("POST", "an automation with that name already exists");
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Stalled-deal nudge")).toBeTruthy(),
    );

    await userEvent.click(screen.getByRole("button", { name: "Use template" }));
    const dialog = screen.getByRole("dialog");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Create" }),
    );
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      /already exists/,
    );

    await userEvent.click(
      within(dialog).getByRole("button", { name: "Cancel" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Use template" }));
    // The form is re-seeded by its key; the refusal has to be reset by hand, and
    // this is the assertion that says somebody did.
    expect(within(screen.getByRole("dialog")).queryByRole("alert")).toBeNull();
  });

  it("leaves the edit dialog's next opening clean", async () => {
    refusing("PATCH", "somebody else changed this automation");
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Nudge stalled fleet deals")).toBeTruthy(),
    );

    const dialog = await openRowEditor();
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      /somebody else changed this automation/,
    );

    await userEvent.click(
      within(dialog).getByRole("button", { name: "Cancel" }),
    );
    expect(within(await openRowEditor()).queryByRole("alert")).toBeNull();
  });
});
