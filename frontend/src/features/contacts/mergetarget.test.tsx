/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { searchCompanyTargets } from "../../components/companyform";
import { MergeAction } from "../../components/merge";
import { LocaleProvider } from "../../i18n";
import { searchContactsTargets } from "./contactrecordactions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const READ_ONLY = "You can only read this record, so it cannot be kept.";

function row(id: string, name: string, writable: boolean) {
  return {
    id,
    full_name: name,
    display_name: name,
    writable,
    source: "manual",
    captured_by: "u",
    created_at: "2026-10-01T10:00:00Z",
    updated_at: "2026-10-01T10:00:00Z",
  };
}

function stubList(rows: ReturnType<typeof row>[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: rows, has_more: false })),
  );
}

async function openAndSearch(
  searchTargets: Parameters<typeof MergeAction>[0]["searchTargets"],
) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <MergeAction
          label="Merge"
          sourceId="source"
          sourceName="Source"
          searchTargets={searchTargets}
          merge={async (id) => ({ id })}
          invalidate="contacts"
          recordKey="contact"
          survivorRoute={() => ({ name: "home" }) as never}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByTestId("merge-record"));
  await user.type(screen.getByRole("searchbox"), "ann");
  return user;
}

describe("the merge target search", () => {
  it("lets a rep keep a record they own or share a team on, and refuses a read-only one with the reason", async () => {
    stubList([
      row("own", "Own Ann", true),
      row("team", "Team Ann", true),
      row("readonly", "Readonly Ann", false),
    ]);
    const user = await openAndSearch(searchContactsTargets);

    const own = await screen.findByRole("button", { name: /Own Ann/ });
    const team = screen.getByRole("button", { name: /Team Ann/ });
    const readOnly = screen.getByRole("button", { name: /Readonly Ann/ });
    expect(own.hasAttribute("disabled")).toBe(false);
    expect(team.hasAttribute("disabled")).toBe(false);
    expect(readOnly.hasAttribute("disabled")).toBe(true);
    expect(readOnly.textContent).toContain(READ_ONLY);
    expect(own.textContent).not.toContain(READ_ONLY);

    await user.click(team);
    expect(screen.getByTestId("merge-confirm").hasAttribute("disabled")).toBe(
      false,
    );
  });

  it("applies the same rule to company targets", async () => {
    stubList([row("c1", "Open Co", true), row("c2", "Locked Co", false)]);
    await openAndSearch(searchCompanyTargets);

    const locked = await screen.findByRole("button", { name: /Locked Co/ });
    expect(locked.hasAttribute("disabled")).toBe(true);
    expect(locked.textContent).toContain(READ_ONLY);
    expect(
      screen.getByRole("button", { name: /Open Co/ }).hasAttribute("disabled"),
    ).toBe(false);
  });
});
