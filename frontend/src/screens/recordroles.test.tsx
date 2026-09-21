/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
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
