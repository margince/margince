/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider, translate } from "../../i18n";
import { renderWithQueryClient } from "../../testing/queryrender";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import { RelationshipsTab } from "./relationshiprows";

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const CONTACT = "01a02be9-2293-75d2-9dd2-3027d9b63dc2";
const COMPANY = "01a02e25-b6bd-7099-8099-581cbf001b77";
const PROJECT = "01a02e25-c7ce-7099-8099-581cbf001c55";

type RoleKind = "deal_stakeholder" | "project_stakeholder" | "billing_contact";

function seat(role: string, kind: RoleKind = "deal_stakeholder") {
  return {
    id: "rel-1",
    kind,
    ...(kind === "deal_stakeholder"
      ? { deal_id: DEAL }
      : { company_id: COMPANY }),
    ...(kind === "project_stakeholder" ? { project_id: PROJECT } : {}),
    contact_id: CONTACT,
    role,
    is_current_primary: false,
    source: "manual",
    captured_by: "human:u-1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Opens the edit form on one seated role, optionally retypes it, and saves.
// A deal seat opens from the deal's tab, the other kinds from the company's.
async function editSeat(
  locale: Locale,
  stored: string,
  retype?: string,
  kind: RoleKind = "deal_stakeholder",
): Promise<{ shown: string; patched: unknown }> {
  const patched: unknown[] = [];
  stubWithSession(
    {
      "GET /relationships": () =>
        jsonResponse({
          data: [seat(stored, kind)],
          page: { next_cursor: null },
        }),
      [`GET /contacts/${CONTACT}`]: () =>
        jsonResponse({ id: CONTACT, full_name: "Mai Trần" }),
      "PATCH /relationships/rel-1": (body) => {
        patched.push(body);
        return jsonResponse({ ...seat(stored, kind), version: 2 });
      },
    },
    { relationship: ["read", "create", "update", "delete"] },
  );
  const user = userEvent.setup();
  renderWithQueryClient(
    <LocaleProvider initial={locale}>
      <RelationshipsTab
        scope={
          kind === "deal_stakeholder"
            ? { deal_id: DEAL }
            : { company_id: COMPANY }
        }
      />
    </LocaleProvider>,
  );
  await user.click(await screen.findByTestId("edit-record"));
  const dialog = within(await screen.findByRole("dialog"));
  const role = dialog.getByRole("textbox", {
    name: translate(locale, "rel.role"),
  });
  const shown = role instanceof HTMLInputElement ? role.value : "";
  if (retype !== undefined) {
    await user.clear(role);
    await user.type(role, retype);
  }
  await user.click(
    dialog.getByRole("button", { name: translate(locale, "record.save") }),
  );
  await waitFor(() => expect(patched).toHaveLength(1));
  return { shown, patched: patched[0] };
}

describe("editing a stakeholder's role", () => {
  it.each(["en", "de", "vi"] as const)(
    "shows the role in the reader's words and keeps its key (%s)",
    async (locale) => {
      const { shown, patched } = await editSeat(locale, "champion");
      expect(shown).toBe(translate(locale, "co.roleLabel.champion"));
      expect(patched).toMatchObject({ role: "champion" });
    },
  );

  it.each(["en", "de", "vi"] as const)(
    "saves a deal role typed in any case and spacing by its key (%s)",
    async (locale) => {
      const label = translate(locale, "co.roleLabel.economic_buyer");
      const { patched } = await editSeat(
        locale,
        "champion",
        ` ${label.toUpperCase().replaceAll(" ", "  ")} `,
      );
      expect(patched).toMatchObject({ role: "economic_buyer" });
    },
  );

  // A free-text role shows with spaces for its underscores. Saved untouched,
  // it keeps the spelling it was stored with.
  it("keeps a free-text role it shows in other words", async () => {
    const { shown, patched } = await editSeat("en", "technical_lead");
    expect(shown).toBe("technical lead");
    expect(patched).toMatchObject({ role: "technical_lead" });
  });

  // A role typed before the vocabulary may spell a deal role's words.
  it.each(["Champion", "economic buyer"])(
    "keeps the stored spelling %s when saved untouched",
    async (stored) => {
      const { shown, patched } = await editSeat("en", stored);
      expect(shown).toBe(stored);
      expect(patched).toMatchObject({ role: stored });
    },
  );
});

// The server judges these roles against closed lists, so a label sent back
// as typed would be refused.
describe("editing a project or billing role from a company", () => {
  it.each(["en", "de", "vi"] as const)(
    "shows a project role in the reader's words and keeps its key (%s)",
    async (locale) => {
      const { shown, patched } = await editSeat(
        locale,
        "delivery_lead",
        undefined,
        "project_stakeholder",
      );
      expect(shown).toBe(translate(locale, "project.role.delivery_lead"));
      expect(patched).toMatchObject({ role: "delivery_lead" });
    },
  );

  it.each(["en", "de", "vi"] as const)(
    "saves a project role typed in the reader's words by its key (%s)",
    async (locale) => {
      const { patched } = await editSeat(
        locale,
        "sponsor",
        translate(locale, "project.role.project_lead"),
        "project_stakeholder",
      );
      expect(patched).toMatchObject({ role: "project_lead" });
    },
  );

  it.each(["en", "de", "vi"] as const)(
    "shows a billing capacity in the reader's words and keeps its key (%s)",
    async (locale) => {
      const { shown, patched } = await editSeat(
        locale,
        "accounts_payable",
        translate(locale, "billing.role.approver"),
        "billing_contact",
      );
      expect(shown).toBe(translate(locale, "billing.role.accountsPayable"));
      expect(patched).toMatchObject({ role: "approver" });
    },
  );
});
