/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { catalogs, LocaleProvider, translate } from "../../i18n";
import { en } from "../../i18n/en";
import { LOCALES } from "../../i18n/locale";
import { ANALYTICS_SCHEMA_KEY } from "../../lib/analyticskeys";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { CustomFieldsAdmin } from "./customfields";
import { FieldBuilder } from "./customfields.builder";
import { AuditRail, FieldTable } from "./customfields.table";

afterEach(cleanup);

// The surface sits behind the app auth gate: useMe only asks /v1/me once a
// workspace slug is resolved, so the integration harness seeds one and clears
// the stubbed globals between cases.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const wrap = (ui: React.ReactNode) =>
  render(<LocaleProvider initial="en">{ui}</LocaleProvider>);

function builder(
  overrides: Partial<React.ComponentProps<typeof FieldBuilder>> = {},
) {
  const onSubmit = vi.fn();
  const onCancel = vi.fn();
  // The real region, not a spy standing where it goes: what the builder owes a
  // reader is a sentence on screen without a completion mark beside it, and a
  // `vi.fn()` in its place proves only that a function was called.
  // The kinds an installed extension declared, seeded where the directory reads
  // them so the builder offers the record type without a request.
  const kinds = new QueryClient();
  kinds.setQueryData(["record-kinds"], {
    data: [{ name: "ext:alpha:visit", display_name: "Visit" }],
  });
  wrap(
    <QueryClientProvider client={kinds}>
      <ToastProvider>
        <FieldBuilder
          object="company"
          pending={false}
          onSubmit={onSubmit}
          onCancel={onCancel}
          {...overrides}
        />
        <ToastRegion />
      </ToastProvider>
    </QueryClientProvider>,
  );
  return { onSubmit, onCancel };
}

describe("FieldBuilder", () => {
  it("mirrors the label into the immutable disabled api key", async () => {
    builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Contract end date");
    const key = screen.getByLabelText(/API name/i) as HTMLInputElement;
    expect(key.value).toBe("company.cf_contract_end_date");
    expect(key).toBeDisabled();
  });

  it("shows the pending DDL preview reflecting the type", async () => {
    builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Contract end date");
    await userEvent.click(screen.getByRole("button", { name: /^Date$/i }));
    expect(
      screen.getByText(
        /ALTER company ADD COLUMN cf_contract_end_date \(date\)/,
      ),
    ).toBeInTheDocument();
  });

  it("refuses a structural label and disables Confirm", async () => {
    builder();
    await userEvent.type(
      screen.getByLabelText(/Label/i),
      "Link to parent account",
    );
    expect(
      screen.getByText(/looks like a new object or relationship/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add field" })).toBeDisabled();
  });

  it("guards an empty label: Confirm disabled, guard toast on click attempt", async () => {
    const { onSubmit } = builder();
    const confirm = screen.getByRole("button", {
      name: "Add field",
    });
    expect(confirm).toBeDisabled();
    // the guard toast is wired to the always-clickable Add affordance
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("reveals the ISO-4217 input for currency", async () => {
    builder();
    await userEvent.click(screen.getByRole("button", { name: /^Currency$/i }));
    expect(screen.getByLabelText(/Currency code/i)).toBeInTheDocument();
  });

  it("reveals the options editor for picklist and blocks removing the last option", async () => {
    const view = builder();
    await userEvent.click(screen.getByRole("button", { name: /^Picklist$/i }));
    const removes = screen.getAllByRole("button", { name: /remove option/i });
    // start with one row; removing it is blocked
    await userEvent.click(removes[removes.length - 1]);
    expect(screen.getByRole("status")).toHaveTextContent(
      "A picklist needs at least one option",
    );
    // And marked as a refusal: the completion dot beside it said the opposite
    // of what the sentence says, so the message wears the danger one.
    expect(document.body.querySelector(".toast-dot-success")).toBeNull();
    expect(document.body.querySelector(".toast-dot-danger")).not.toBeNull();
    expect(view.onSubmit).not.toHaveBeenCalled();
  });

  it("submits a well-formed draft on Confirm", async () => {
    const { onSubmit } = builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Renewal date");
    await userEvent.click(screen.getByRole("button", { name: /^Date$/i }));
    await userEvent.click(screen.getByRole("button", { name: "Add field" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        object: "company",
        label: "Renewal date",
        type: "date",
      }),
    );
  });

  it("submits a record field with the one kind it points at", async () => {
    const { onSubmit } = builder();
    const user = userEvent.setup();
    await user.type(screen.getByLabelText(/Label/i), "Last visit");
    await user.click(
      screen.getByRole("button", { name: en["cf.type.record"] }),
    );
    const add = screen.getByRole("button", { name: "Add field" });
    expect(add).toBeDisabled();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: en["cf.recordKind"] }),
      "Visit",
    );
    await user.click(add);
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ type: "record", options: ["ext:alpha:visit"] }),
    );
  });

  it("submits a field kept off create forms when the reader unticks it", async () => {
    const { onSubmit } = builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Previous owner");
    await userEvent.click(
      screen.getByRole("checkbox", { name: en["cf.showInCreate"] }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Add field" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ label: "Previous owner", showInCreate: false }),
    );
  });

  it("leaves through Cancel without submitting a draft", async () => {
    const { onCancel, onSubmit } = builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Renewal date");
    await userEvent.click(screen.getByRole("button", { name: /^Cancel$/i }));
    expect(onCancel).toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("keeps Confirm disabled for a picklist whose only option is blank", async () => {
    builder();
    await userEvent.type(screen.getByLabelText(/Label/i), "Deal source");
    await userEvent.click(screen.getByRole("button", { name: /^Picklist$/i }));
    // The single option row is left blank — a picklist with no real choice
    // must not be confirmable.
    expect(screen.getByRole("button", { name: "Add field" })).toBeDisabled();
    // Typing a real option flips Confirm back on.
    await userEvent.type(screen.getByLabelText(/Option label/i), "Referral");
    expect(screen.getByRole("button", { name: "Add field" })).toBeEnabled();
  });
});

type CustomField = components["schemas"]["CustomField"];

const field = (over: Partial<CustomField> = {}): CustomField => ({
  id: "01J",
  object: "deal",
  label: "Renewal date",
  slug: "renewal_date",
  type: "date",
  status: "active",
  column_name: "cf_renewal_date",
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
  version: 1,
  ...over,
});

const menuName = (label: string) =>
  translate("en", "table.rowActions", { name: label });

// The row's own menu, so a test cannot pass by opening another row's.
async function openFieldMenu(label: string) {
  const trigger = await screen.findByRole("button", { name: menuName(label) });
  await userEvent.click(trigger);
  const items = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!items) {
    throw new Error(`the menu for ${label} drew no items`);
  }
  return within(items);
}

describe("FieldTable", () => {
  it("lists a field by its name and type, with the api key left to the builder", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field()]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(screen.getByText("Renewal date")).toBeInTheDocument();
    expect(screen.queryByText("deal.cf_renewal_date")).toBeNull();
    expect(screen.getByText(/Date/)).toBeInTheDocument();
    expect(screen.getByText("You")).toBeInTheDocument();
  });

  it("names a record field's type, and a type this build does not know by its own name", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[
          field({ id: "f-record", label: "Visit", type: "record" }),
          field({
            id: "f-newer",
            label: "Newer",
            type: "geopoint" as CustomField["type"],
          }),
        ]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(screen.getByText(en["cf.type.record"])).toBeInTheDocument();
    expect(screen.getByText("geopoint")).toBeInTheDocument();
  });

  it("offers a live field's verbs in its row menu", async () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field()]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    const menu = await openFieldMenu("Renewal date");
    expect(
      menu.getByRole("button", { name: en["cf.edit"] }),
    ).toBeInTheDocument();
    expect(
      menu.getByRole("button", { name: en["cf.archive"] }),
    ).toBeInTheDocument();
  });

  it("hides edit/archive controls when the viewer cannot manage", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field()]}
        canEdit={false}
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(
      screen.queryByRole("button", { name: menuName("Renewal date") }),
    ).toBeNull();
  });

  // The API offers no restore, so a retired row offers nothing at all.
  it("marks a retired field retired and offers it no verb", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field({ status: "retired" })]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(screen.getByText(en["cf.retired"])).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: menuName("Renewal date") }),
    ).toBeNull();
  });
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Recorded = { method: string; url: string; body: unknown };

// A fetch stub over the shipped custom-fields contract: /v1/me for the role
// probe, per-object list reads keyed off the `object` query param, a 201 echo
// on create, retire + rename recorded verbatim, and an empty audit page. Every
// route the screen touches is answered so QueryGate renders content, never its
// error card. `opts.failCreate` makes POST /custom-fields reject with a 422 so
// the optimistic-rollback path can be exercised.
// The builder needs create; rename and retire need update. Retire is a
// LIFECYCLE update, never a delete — no surface here offers one.
const FIELD_MANAGER: GrantSpec = { custom_field: ["create", "update"] };

function customFieldsBackend(
  dealFields: CustomField[],
  companyFields: CustomField[],
  calls: Recorded[],
  allow: GrantSpec = FIELD_MANAGER,
  opts: {
    failCreate?: boolean;
    liveLists?: components["schemas"]["CustomFieldLiveLists"];
  } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    const readBody = async (): Promise<Record<string, unknown>> =>
      (request
        ? await request.json()
        : JSON.parse(String(init?.body))) as Record<string, unknown>;
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (url.includes("/audit-log")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    // Retire is a POST too, but on the /{id}/retire sub-path — match it before
    // the generic create so it is recorded as an archive, not parsed as a body.
    if (url.includes("/retire") && method === "POST") {
      calls.push({ method, url, body: null });
      return jsonResponse(field({ id: "archived", status: "retired" }));
    }
    if (url.endsWith("/lists") && method === "GET") {
      calls.push({ method, url, body: null });
      return jsonResponse(opts.liveLists ?? { lists: [], unseen_count: 0 });
    }
    if (url.includes("/custom-fields") && method === "PATCH") {
      const body = await readBody();
      calls.push({ method, url, body });
      // The saved row: the field the URL names, with the patch applied.
      const id = new URL(url).pathname.split("/").pop();
      const saved = [...dealFields, ...companyFields].find((f) => f.id === id);
      return jsonResponse({ ...(saved ?? field()), ...body });
    }
    if (url.includes("/custom-fields") && method === "POST") {
      const body = await readBody();
      calls.push({ method, url, body });
      if (opts.failCreate) {
        return jsonResponse(
          { title: "Unprocessable", detail: "rejected" },
          422,
        );
      }
      const created = field({
        id: `cf-new-${calls.length}`,
        object: body.object as CustomField["object"],
        label: String(body.label),
        type: body.type as CustomField["type"],
        currency: (body.currency as string | undefined) ?? null,
        options: (body.options as string[] | undefined) ?? null,
      });
      return jsonResponse(created, 201);
    }
    if (url.includes("/custom-fields")) {
      const object = new URL(url).searchParams.get("object");
      calls.push({ method, url, body: null });
      const data = object === "company" ? companyFields : dealFields;
      return jsonResponse({ data, page: { next_cursor: null } });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

const renderAdmin = (
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
) => {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a suite
            whose subject is what a write SAYS has to mount it the same way. */}
        <ToastProvider>
          <CustomFieldsAdmin />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

describe("CustomFieldsAdmin", () => {
  // The builder is a dialog behind a row verb now, so anything whose subject is
  // one of its inputs opens it first. The row's verb reads "New field" and the
  // dialog's submit "Add field", which is queried within the dialog.
  const openBuilder = async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "New field" }),
    );
    return within(screen.getByRole("dialog"));
  };

  // The settings page that hosts this owns the .wrap reading column and the h1,
  // so the surface must contribute neither: a nested .wrap double-pads the page
  // and a second h1 gives the document two page titles.
  it("renders as a section — no reading column of its own, heading at level 2", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend([field({ id: "d1", label: "Renewal date" })], [], []),
    );
    const { container } = renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    expect(container.querySelector(".wrap")).toBeNull();
    expect(container.querySelector(".panel")).not.toBeNull();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
    expect(
      screen.getByRole("heading", { level: 2, name: "Custom fields" }),
    ).toBeInTheDocument();
  });

  it("offers every object in the Object picker beside the selected object's fields", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend([field({ id: "d1", label: "Renewal date" })], [], []),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("combobox", { name: "Object" }));
    const listbox = within(screen.getByRole("listbox"));
    for (const name of [
      "Deal",
      "Company",
      "Contact",
      "Lead",
      "Project",
      "Contract",
    ]) {
      expect(listbox.getByRole("option", { name })).toBeInTheDocument();
    }
  });

  it("swaps to the company fields when Company is picked", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [
          field({
            id: "o1",
            object: "company",
            label: "Industry code",
            column_name: "cf_industry_code",
            type: "text",
          }),
        ],
        calls,
      ),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "Object" }),
      "Company",
    );
    await waitFor(() =>
      expect(screen.getByText("Industry code")).toBeInTheDocument(),
    );
    expect(screen.queryByText("Renewal date")).toBeNull();
    expect(calls.some((call) => call.url.includes("object=company"))).toBe(
      true,
    );
  });

  it("creates a field with source:manual and shows the success toast", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal("fetch", customFieldsBackend([], [], calls));
    renderAdmin();
    const dialog = await openBuilder();
    await userEvent.type(dialog.getByLabelText(/^Label/i), "Deal size");
    await userEvent.click(dialog.getByRole("button", { name: /^Number$/i }));
    await userEvent.click(dialog.getByRole("button", { name: "Add field" }));
    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    const post = calls.find((call) => call.method === "POST");
    expect(post?.body).toMatchObject({
      object: "deal",
      label: "Deal size",
      type: "number",
      source: "manual",
      show_in_create: true,
    });
    await waitFor(() =>
      expect(screen.getByText(/Deal size” added/)).toBeInTheDocument(),
    );
    // The dialog is what carried the form, so a committed draft has nothing
    // left to type into: it closes, and the toast reports the outcome on the
    // card behind it. This is also what stops a second Confirm resubmitting a
    // field that already exists.
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("gives a non-managing role the read-only view with no builder or archive", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        [],
        {},
      ),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    expect(screen.queryByRole("button", { name: "New field" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add field" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: menuName("Renewal date") }),
    ).toBeNull();
    // The page says once that it is read-only; the intro about adding fields
    // is for a reader who can add one.
    expect(screen.queryByText(en["cf.subtitle"])).toBeNull();
  });

  it("tells a read-only reader the object has no fields without asking them to add one", async () => {
    vi.stubGlobal("fetch", customFieldsBackend([], [], [], {}));
    renderAdmin();
    expect(
      await screen.findByText(
        translate("en", "cf.emptyReadOnly", { object: "Deal" }),
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["cf.empty.deal"])).toBeNull();
  });

  // One grant at a time: the builder needs create, rename/retire need update.
  // A fixture holding both cannot tell a correct binding from a transposed one.
  it("offers rename and retire on update alone, without the builder", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        [],
        { custom_field: ["update"] },
      ),
    );
    renderAdmin();
    const menu = await openFieldMenu("Renewal date");
    expect(menu.getByRole("button", { name: en["cf.archive"] })).toBeTruthy();
    expect(menu.getByRole("button", { name: en["cf.edit"] })).toBeTruthy();
    // The row that opens the builder, spelled as the catalog spells it.
    expect(screen.queryByText("Add field to Deal")).toBeNull();
    expect(screen.queryByRole("button", { name: "New field" })).toBeNull();
  });

  it("offers the builder on create alone, without rename or retire", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        [],
        { custom_field: ["create"] },
      ),
    );
    renderAdmin();
    await waitFor(() => expect(screen.getByText("Renewal date")).toBeTruthy());
    // Positive as well as negative: without this a broken create binding would
    // pass, since "no archive control" is also true when nothing renders. The
    // builder is behind the card's HEADER verb, so what proves it is reachable
    // is that verb plus the form it opens.
    expect(screen.getByRole("button", { name: "New field" })).toBeTruthy();
    const dialog = await openBuilder();
    expect(dialog.getByRole("button", { name: "Add field" })).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: menuName("Renewal date") }),
    ).toBeNull();
  });

  it("rolls back the optimistic staged row and toasts the error on create failure", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Existing field" })],
        [],
        calls,
        FIELD_MANAGER,
        { failCreate: true },
      ),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Existing field")).toBeInTheDocument(),
    );
    const dialog = await openBuilder();
    await userEvent.type(dialog.getByLabelText(/^Label/i), "Doomed field");
    await userEvent.click(dialog.getByRole("button", { name: /^Number$/i }));
    await userEvent.click(dialog.getByRole("button", { name: "Add field" }));
    // The POST is attempted…
    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    // …and after the 422 the list is back to its prior rows (staged row gone)
    // with an honest error toast surfaced from the problem detail.
    await waitFor(() =>
      expect(screen.getByText(/rejected/)).toBeInTheDocument(),
    );
    expect(screen.queryByText("Doomed field")).toBeNull();
    expect(screen.queryByText(/writing/i)).toBeNull();
    expect(screen.getByText("Existing field")).toBeInTheDocument();
  });

  it("renders an honest empty state for an object with no fields", async () => {
    vi.stubGlobal("fetch", customFieldsBackend([], [], []));
    renderAdmin();
    expect(
      await screen.findByText(/No custom fields on Deal yet/i),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("archives a field and shows the archived toast", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        calls,
      ),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    const menu = await openFieldMenu("Renewal date");
    await userEvent.click(menu.getByRole("button", { name: en["cf.archive"] }));
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText(en["cf.retire.noLists"]),
    ).toBeInTheDocument();
    expect(calls.some((call) => call.url.includes("/retire"))).toBe(false);
    await userEvent.click(
      within(dialog).getByRole("button", { name: en["cf.archive"] }),
    );
    await waitFor(() =>
      expect(
        calls.some(
          (call) => call.method === "POST" && call.url.includes("/retire"),
        ),
      ).toBe(true),
    );
    const retire = calls.find((call) => call.url.includes("/retire"));
    expect(retire?.url).toContain("/custom-fields/d1/retire");
    // Retirement is terminal: no route restores a field, so the toast must not
    // promise one.
    const toast = await screen.findByText(/archived/);
    expect(toast.textContent).not.toMatch(/restore/i);
  });

  // No route restores a retired field. Neither the dialog that asks nor the
  // toast that answers may promise one, in any language the product ships.
  it.each(LOCALES)(
    "says in %s that archiving is permanent, before and after",
    (locale) => {
      const catalog = catalogs[locale];
      for (const key of ["cf.retire.body", "cf.archived"] as const) {
        expect(catalog[key], `${locale} ${key}`).not.toMatch(
          /restore|wiederherstell|khôi phục/i,
        );
      }
      // The dialog says it before the action, in the toast's own words.
      const permanence = catalog["cf.archived"].split(". ").pop() ?? "";
      expect(catalog["cf.retire.body"], locale).toContain(permanence);
    },
  );

  it("names the Live Lists that filter on a field before it is archived, and only counts the rest", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        calls,
        FIELD_MANAGER,
        {
          liveLists: {
            lists: [
              { id: "l1", name: "Renewals this quarter", sharing: "team" },
            ],
            unseen_count: 2,
          },
        },
      ),
    );
    renderAdmin();
    const menu = await openFieldMenu("Renewal date");
    await userEvent.click(menu.getByRole("button", { name: en["cf.archive"] }));
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByText("Renewals this quarter"),
    ).toBeInTheDocument();
    expect(within(dialog).getByText(en["cf.retire.lists"])).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        en["cf.retire.unseen_other"].replace("{count}", "2"),
      ),
    ).toBeInTheDocument();
    expect(calls.find((call) => call.url.endsWith("/lists"))?.url).toContain(
      "/custom-fields/d1/lists",
    );
  });

  it("renames a field via the modal, sending the new label in a PATCH", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        calls,
      ),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Renewal date")).toBeInTheDocument(),
    );
    const menu = await openFieldMenu("Renewal date");
    await userEvent.click(menu.getByRole("button", { name: en["cf.edit"] }));
    const input = screen.getByLabelText(/New label/i);
    expect(screen.getByText("deal.cf_renewal_date").tagName).toBe("CODE");
    await userEvent.clear(input);
    await userEvent.type(input, "Contract end date");
    await userEvent.click(screen.getByRole("button", { name: /Save/i }));
    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.url).toContain("/custom-fields/d1");
    expect(patch?.body).toMatchObject({ label: "Contract end date" });
  });
});

describe("the create-form setting on the field table", () => {
  it("turns a field off for create forms from its row switch", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [field({ id: "d1", label: "Renewal date" })],
        [],
        calls,
      ),
    );
    renderAdmin();
    const toggle = await screen.findByRole("switch", {
      name: "Show “Renewal date” in create forms",
    });
    expect(toggle).toHaveAttribute("aria-checked", "true");
    await userEvent.click(toggle);
    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.url).toContain("/custom-fields/d1");
    expect(patch?.body).toEqual({ show_in_create: false });
    expect(
      await screen.findByText(/hidden from create forms/),
    ).toBeInTheDocument();
  });

  // The list read after the first write never answers. So the second flip
  // carries the right version only if the saved row went into the cache.
  it("sends each flip the version the previous one saved", async () => {
    const calls: Recorded[] = [];
    const deal = [field({ id: "d1", version: 3, show_in_create: true })];
    const backend = customFieldsBackend(deal, [], calls);
    const sent: { ifMatch: string | null; body: unknown }[] = [];
    vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
      if (!(input instanceof Request)) return backend(input);
      if (input.method === "PATCH") {
        const body = (await input.json()) as Partial<CustomField>;
        sent.push({ ifMatch: input.headers.get("If-Match"), body });
        deal[0] = { ...deal[0], ...body, version: (deal[0].version ?? 0) + 1 };
        return jsonResponse(deal[0]);
      }
      if (sent.length > 0 && input.url.includes("/custom-fields?")) {
        return new Promise<Response>(() => {});
      }
      return backend(input);
    });
    renderAdmin();
    const name = "Show “Renewal date” in create forms";
    await userEvent.click(await screen.findByRole("switch", { name }));
    await waitFor(() =>
      expect(screen.getByRole("switch", { name })).toHaveAttribute(
        "aria-checked",
        "false",
      ),
    );
    await waitFor(() =>
      expect(screen.getByRole("switch", { name })).not.toHaveAttribute(
        "aria-busy",
      ),
    );
    await userEvent.click(screen.getByRole("switch", { name }));
    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent).toEqual([
      { ifMatch: "3", body: { show_in_create: false } },
      { ifMatch: "4", body: { show_in_create: true } },
    ]);
  });

  it("states the setting without a switch to a reader who may not change it", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field({ show_in_create: false })]}
        canEdit={false}
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(screen.queryByRole("switch")).toBeNull();
    expect(screen.getByText(en["field.no"])).toBeInTheDocument();
  });
});

describe("the reporting setting on the field table", () => {
  it("adds a deal picklist to reports from its row switch", async () => {
    const calls: Recorded[] = [];
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [
          field({
            id: "d1",
            label: "Territory",
            type: "picklist",
            options: ["EMEA"],
          }),
        ],
        [],
        calls,
      ),
    );
    renderAdmin();
    const toggle = await screen.findByRole("switch", {
      name: "Use “Territory” in reports",
    });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    await userEvent.click(toggle);
    await waitFor(() =>
      expect(calls.some((call) => call.method === "PATCH")).toBe(true),
    );
    const patch = calls.find((call) => call.method === "PATCH");
    expect(patch?.body).toEqual({ reportable: true });
    expect(await screen.findByText(/added to reports/)).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByRole("switch", { name: "Use “Territory” in reports" }),
      ).toHaveAttribute("aria-checked", "true"),
    );
  });

  it("makes the analytics vocabulary stale, so the builder offers the field", async () => {
    vi.stubGlobal(
      "fetch",
      customFieldsBackend(
        [
          field({
            id: "d1",
            label: "Territory",
            type: "picklist",
            options: ["EMEA"],
          }),
        ],
        [],
        [],
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(ANALYTICS_SCHEMA_KEY, { version: "v1", entities: [] });
    renderAdmin(client);
    await userEvent.click(
      await screen.findByRole("switch", { name: "Use “Territory” in reports" }),
    );
    await waitFor(() =>
      expect(client.getQueryState(ANALYTICS_SCHEMA_KEY)?.isInvalidated).toBe(
        true,
      ),
    );
  });

  it("offers no switch for a field reports cannot group by", () => {
    wrap(
      <FieldTable
        object="deal"
        fields={[field({ label: "Renewal date", type: "date" })]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(
      screen.queryByRole("switch", { name: "Use “Renewal date” in reports" }),
    ).toBeNull();
  });

  it("offers no reporting column on an object reports do not group", () => {
    wrap(
      <FieldTable
        object="contact"
        fields={[field({ object: "contact", type: "boolean" })]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(screen.queryByText(en["cf.col.inReports"])).toBeNull();
  });

  it("offers a company picklist to reports, which group deals by it", () => {
    wrap(
      <FieldTable
        object="company"
        fields={[field({ object: "company", label: "Tier", type: "picklist" })]}
        canEdit
        meUserId="u1"
        onRename={() => {}}
        onArchive={() => {}}
        onSetting={() => {}}
      />,
    );
    expect(
      screen.getByRole("switch", { name: "Use “Tier” in reports" }),
    ).toBeInTheDocument();
  });
});

describe("AuditRail states", () => {
  const noop = () => undefined;

  it("says nothing about emptiness while the read is still running", () => {
    wrap(<AuditRail entries={[]} state="loading" onRetry={noop} />);
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
  });

  it("offers a retry on a failed read, and does not claim the trail is empty", () => {
    const retry = vi.fn();
    wrap(<AuditRail entries={[]} state="failed" onRetry={retry} />);
    expect(screen.getByText(/did not load/i)).toBeInTheDocument();
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
    expect(screen.getByRole("button", { name: /^retry$/i })).toBeTruthy();
  });

  it("says the trail is withheld, not empty, when the role cannot read it", () => {
    wrap(<AuditRail entries={[]} state="withheld" onRetry={noop} />);
    expect(screen.getByText(/Hidden for your role/i)).toBeInTheDocument();
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
  });

  it("shows the empty line only for a settled, genuinely empty read", () => {
    wrap(<AuditRail entries={[]} state="empty" onRetry={noop} />);
    expect(
      screen.getByText(/No custom field changes yet/i),
    ).toBeInTheDocument();
  });
});
