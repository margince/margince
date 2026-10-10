/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { en } from "../i18n/en";
import { RecordRolesCard } from "./recordroles";
import { jsonResponse, render } from "./settings.testkit";

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it.each([
  {
    label: "Renewal coordinator",
    records: ["Company"],
    kinds: ["Colleague"],
    record_types: ["company"],
    assignee_kinds: ["user"],
  },
  {
    label: "Implementation team",
    records: ["Deal", "Project"],
    kinds: ["Team"],
    record_types: ["deal", "project"],
    assignee_kinds: ["team"],
  },
])("creates $label with only the selected applicability", async (example) => {
  const posted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.url.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            roles: ["admin"],
            allow: { custom_field: ["read", "create"] },
          }),
        );
      }
      if (request.url.endsWith("/record-roles")) {
        if (request.method === "POST") {
          const body = await request.json();
          posted.push(body);
          return jsonResponse({ id: "new-role", ...body }, 201);
        }
        return jsonResponse({ data: [] });
      }
      throw new Error(`Unexpected request: ${request.method} ${request.url}`);
    }),
  );
  const user = userEvent.setup();
  render(<RecordRolesCard />);
  await user.click(await screen.findByRole("button", { name: "Add role" }));
  const dialog = within(screen.getByRole("dialog"));
  const save = dialog.getByRole("button", { name: "Add role" });
  await user.type(dialog.getByRole("textbox", { name: "Name" }), example.label);
  expect(save).toBeDisabled();
  for (const name of example.records) {
    await user.click(dialog.getByRole("checkbox", { name }));
  }
  expect(save).toBeDisabled();
  for (const name of example.kinds) {
    await user.click(dialog.getByRole("checkbox", { name }));
  }
  expect(save).toBeEnabled();
  await user.click(dialog.getByRole("checkbox", { name: example.kinds[0] }));
  expect(save).toBeDisabled();
  await user.click(dialog.getByRole("checkbox", { name: example.kinds[0] }));
  await user.click(save);
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  expect(posted).toEqual([
    {
      label: example.label,
      record_types: example.record_types,
      assignee_kinds: example.assignee_kinds,
    },
  ]);
});

const ADMIN: GrantSpec = { custom_field: ["read", "create", "update"] };

const CONFLICT = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

const TECHNICAL_CONTACT = {
  id: "role-technical_contact",
  key: "technical_contact",
  label: "Technical contact",
  record_types: ["company", "deal", "project"],
  assignee_kinds: ["user", "team"],
  sort_order: 10,
  active: true,
  system: true,
  version: 1,
  created_at: "2026-08-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

type Call = { method: string; url: string; body: unknown };

function rolesBackend(
  allow: GrantSpec,
  calls: Call[] = [],
  refuseWrite = false,
) {
  return vi.fn(async (request: Request) => {
    const raw = request.method === "GET" ? "" : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
    });
    if (request.url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method !== "GET") {
      return refuseWrite
        ? jsonResponse(CONFLICT, 409)
        : jsonResponse(TECHNICAL_CONTACT);
    }
    return jsonResponse({
      data: [
        TECHNICAL_CONTACT,
        {
          ...TECHNICAL_CONTACT,
          id: "role-delivery_lead",
          key: "delivery_lead",
          label: "Delivery lead",
          record_types: ["project"],
          assignee_kinds: ["user"],
        },
      ],
    });
  });
}

function patched(calls: Call[], path: string, body: unknown) {
  return calls.some(
    (call) =>
      call.method === "PATCH" &&
      call.url.endsWith(path) &&
      JSON.stringify(call.body) === JSON.stringify(body),
  );
}

it("says where a role applies and who holds it in words, never the stored keys", async () => {
  vi.stubGlobal("fetch", rolesBackend(ADMIN));
  render(<RecordRolesCard />);
  const row = await screen.findByTestId("record-role-technical_contact");
  expect(within(row).getByText("Technical contact")).toBeInTheDocument();
  expect(within(row).getByText("technical_contact")).toBeInTheDocument();
  expect(
    within(row).getByText("On companies, deals and projects"),
  ).toBeInTheDocument();
  expect(
    within(row).getByText(en["recordRoles.heldBy.either"]),
  ).toBeInTheDocument();
  expect(within(row).queryByText(/company, deal/)).toBeNull();
  expect(within(row).queryByRole("textbox")).toBeNull();
  const lead = screen.getByTestId("record-role-delivery_lead");
  expect(within(lead).getByText("On projects")).toBeInTheDocument();
  expect(
    within(lead).getByText(en["recordRoles.heldBy.user"]),
  ).toBeInTheDocument();
});

it("renames a role through its menu's dialog and retires one through its switch", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", rolesBackend(ADMIN, calls));
  render(<RecordRolesCard />);
  await user.click(
    await screen.findByRole("button", {
      name: en["leadSources.rowActions"].replace("{label}", "Delivery lead"),
    }),
  );
  await user.click(screen.getByRole("button", { name: "Rename" }));
  const dialog = within(await screen.findByRole("dialog"));
  const field = dialog.getByLabelText(en["recordRoles.addLabel"]);
  expect(field).toHaveValue("Delivery lead");
  await user.clear(field);
  await user.type(field, "Project lead{Enter}");
  await waitFor(() =>
    expect(
      patched(calls, "/record-roles/role-delivery_lead", {
        label: "Project lead",
      }),
    ).toBe(true),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await user.click(
    screen.getByRole("switch", { name: "Delivery lead can be newly assigned" }),
  );
  await waitFor(() =>
    expect(
      patched(calls, "/record-roles/role-delivery_lead", { active: false }),
    ).toBe(true),
  );
});

it("says a duplicate name on the Name field and keeps the dialog open", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", rolesBackend(ADMIN, [], true));
  render(<RecordRolesCard />);
  await user.click(await screen.findByRole("button", { name: "Add role" }));
  const dialog = within(screen.getByRole("dialog"));
  const field = dialog.getByRole("textbox", { name: "Name" });
  await user.type(field, "Technical contact");
  await user.click(dialog.getByRole("checkbox", { name: "Deal" }));
  await user.click(dialog.getByRole("checkbox", { name: "Team" }));
  await user.click(dialog.getByRole("button", { name: "Add role" }));
  expect(await dialog.findByText(en["recordRoles.duplicate"])).toBeTruthy();
  expect(field.getAttribute("aria-invalid")).toBe("true");
  await user.type(field, "s");
  expect(dialog.queryByText(en["recordRoles.duplicate"])).toBeNull();
});

it("leaves every control inert for a reader and says why", async () => {
  vi.stubGlobal("fetch", rolesBackend({ custom_field: ["read"] }));
  render(<RecordRolesCard />);
  expect(
    await screen.findByRole("switch", {
      name: "Technical contact can be newly assigned",
    }),
  ).toBeDisabled();
  expect(screen.getByText(en["leadSources.readOnlyTitle"])).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Add role" })).toBeNull();
  expect(
    screen.queryByRole("button", {
      name: en["leadSources.rowActions"].replace(
        "{label}",
        "Technical contact",
      ),
    }),
  ).toBeNull();
});
