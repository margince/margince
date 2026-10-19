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
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AutomationsAdmin } from "./automations";
import { paramFields } from "./automations.params";

// The renewal reminder's parameters, split from automations.test.tsx: the
// date-field picker, the record-type choice and the yearly checkbox.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
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

type Recorded = {
  method: string;
  url: string;
  body: unknown;
  ifMatch: string | null;
};

const AUTOMATION_OPERATOR: GrantSpec = {
  automation: ["create", "read", "update", "delete"],
};

async function openCreateDialog(index = 0): Promise<HTMLElement> {
  await userEvent.click(
    screen.getAllByRole("button", { name: /^Use template/ })[index],
  );
  return screen.getByRole("dialog");
}

// renewal_reminder's real catalog schema. days_before is the one integer param.
// object and date_field name the workspace's own cf_* column to watch, and
// recurs_yearly opts a stored value into yearly re-arming. The closed reader in
// automations.params.ts handles both, not a special case in the component.
type CustomField = components["schemas"]["CustomField"];

const renewalReminderSchema = {
  type: "object",
  properties: {
    date_field: {
      type: "string",
      description: "The workspace's own custom date-field name to watch.",
    },
    days_before: {
      type: "integer",
      minimum: 1,
      maximum: 365,
      default: 30,
    },
    object: {
      type: "string",
      enum: ["contact", "company", "deal", "lead", "project"],
      description: "Which record type owns the watched date field.",
    },
    recurs_yearly: {
      type: "boolean",
      default: false,
      description: "Whether the watched date recurs every year.",
    },
  },
};

const renewalCatalogEntry: CatalogEntry = {
  key: "renewal_reminder",
  name: "Renewal reminder",
  trigger: "clock.daily",
  action: "create_task",
  tier: "auto_execute",
  params_schema: renewalReminderSchema,
};

function customField(overrides: Partial<CustomField>): CustomField {
  return {
    id: "cf-1",
    object: "contact",
    label: "Field",
    slug: "field",
    type: "text",
    status: "active",
    column_name: "cf_field",
    show_in_create: true,
    created_by: "u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const CONTACT_DATE_FIELDS: CustomField[] = [
  customField({
    id: "cf-birthday",
    object: "contact",
    label: "Birthday",
    slug: "birthday",
    type: "date",
    column_name: "cf_birthday",
  }),
  // Filtered out on two grounds (retired, and the wrong type), proving the
  // picker's client-side filter reads both, not just one.
  customField({
    id: "cf-old",
    object: "contact",
    label: "Old renewal date",
    slug: "old-renewal-date",
    type: "date",
    status: "retired",
    column_name: "cf_old_renewal_date",
  }),
  customField({
    id: "cf-name",
    object: "contact",
    label: "Nickname",
    slug: "nickname",
    type: "text",
    column_name: "cf_nickname",
  }),
];

function renewalBackend(calls: Recorded[]) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow: AUTOMATION_OPERATOR }));
    }
    if (url.includes("/automations/catalog")) {
      return jsonResponse({ data: [renewalCatalogEntry] });
    }
    if (url.includes("/custom-fields")) {
      const object = new URL(url).searchParams.get("object");
      return jsonResponse({
        data: CONTACT_DATE_FIELDS.filter((field) => field.object === object),
        page: { next_cursor: null },
      });
    }
    if (url.includes("/automations") && method === "POST") {
      const body: unknown = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      calls.push({ method, url, body, ifMatch: null });
      return jsonResponse(
        {
          id: "au-1",
          ...(body as { key: string; name: string; params: unknown }),
          status: "paused",
          version: 1,
          created_at: "2026-08-13T08:00:00Z",
        },
        201,
      );
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

describe("renewal_reminder's schema-driven params (GH-706)", () => {
  it("paramFields reads days_before/object/recurs_yearly, gives date_field its own picker kind, and object its enum options", () => {
    expect(paramFields(renewalReminderSchema)).toEqual([
      { key: "date_field", kind: "date_field", initial: "" },
      { key: "days_before", kind: "integer", min: 1, max: 365, initial: "30" },
      {
        key: "object",
        kind: "enum",
        initial: "",
        options: ["contact", "company", "deal", "lead", "project"],
      },
      { key: "recurs_yearly", kind: "boolean", initial: "false" },
    ]);
  });

  it("the boolean param renders as a checkbox, and object/date_field render as selects rather than free text", async () => {
    vi.stubGlobal("fetch", renewalBackend([]));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Renewal reminder")).toBeTruthy(),
    );
    await openCreateDialog();

    // recurs_yearly: a checkbox, not a spinbutton or free-text box, and it
    // starts at the schema's own default (false, i.e. unchecked).
    const recurring = screen.getByRole("checkbox", { name: "recurs_yearly" });
    expect(recurring).not.toBeChecked();

    // object is a combobox sourced from the schema's own enum, and date_field
    // is a combobox (the Select control) too. Neither is a native <select> or
    // a textbox a typo could break. days_before stays an ordinary spinbutton.
    expect(screen.getByRole("combobox", { name: "object" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "date_field" })).toBeTruthy();
    expect(
      screen.getByRole("spinbutton", { name: "days_before" }),
    ).toBeTruthy();
  });

  it("the date_field picker is disabled with a hint until an object is chosen, then queries /custom-fields for it", async () => {
    vi.stubGlobal("fetch", renewalBackend([]));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Renewal reminder")).toBeTruthy(),
    );
    await openCreateDialog();

    // No object chosen yet: disabled, with the reason stated rather than a
    // silently broken empty control.
    expect(screen.getByRole("combobox", { name: "date_field" })).toBeDisabled();
    expect(
      screen.getByText("Choose an object first to list its date fields."),
    ).toBeTruthy();

    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "object" }),
      "contact",
    );

    const picker = screen.getByRole("combobox", { name: "date_field" });
    await waitFor(() => expect(picker).not.toBeDisabled());

    // Only the active, date-typed field reaches the list. The retired one and
    // the text-typed one are both filtered out, so the picker reads both checks.
    await userEvent.click(picker);
    const listbox = screen.getByRole("listbox");
    expect(
      within(listbox).getByRole("option", { name: "Birthday" }),
    ).toBeTruthy();
    expect(
      within(listbox).queryByRole("option", { name: "Old renewal date" }),
    ).toBeNull();
    expect(
      within(listbox).queryByRole("option", { name: "Nickname" }),
    ).toBeNull();
  });

  it("disables the date_field picker with a load-error hint when /custom-fields fails, rather than an empty enabled control", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : null;
        const url = String(request ? request.url : input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: AUTOMATION_OPERATOR }));
        }
        if (url.includes("/automations/catalog")) {
          return jsonResponse({ data: [renewalCatalogEntry] });
        }
        if (url.includes("/custom-fields")) {
          return jsonResponse(
            { code: "internal", message: "custom field lookup failed" },
            500,
          );
        }
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }),
    );
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Renewal reminder")).toBeTruthy(),
    );
    await openCreateDialog();

    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "object" }),
      "contact",
    );

    const picker = screen.getByRole("combobox", { name: "date_field" });
    await waitFor(() => expect(picker).toBeDisabled());
    expect(screen.getByText("Could not load date fields. Retry.")).toBeTruthy();
  });

  it("round-trips the boolean and the picked date field through paramsFromValues into the create request", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal("fetch", renewalBackend(calls));
    render(<AutomationsAdmin />);
    await waitFor(() =>
      expect(screen.getByText("Renewal reminder")).toBeTruthy(),
    );
    await openCreateDialog();

    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "object" }),
      "contact",
    );
    await waitFor(() =>
      expect(
        screen.getByRole("combobox", { name: "date_field" }),
      ).not.toBeDisabled(),
    );
    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "date_field" }),
      "Birthday",
    );
    await userEvent.click(
      screen.getByRole("checkbox", { name: "recurs_yearly" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toMatchObject({
      key: "renewal_reminder",
      params: {
        object: "contact",
        date_field: "cf_birthday",
        recurs_yearly: true,
        days_before: 30,
      },
    });
  });
});
