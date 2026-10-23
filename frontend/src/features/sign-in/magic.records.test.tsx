/** @vitest-environment happy-dom */

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  line,
  receipt,
  renderMagic,
} from "../../testing/magic.testkit";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { ChangeList } from "./magic.records";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const LINE = "00000000-0000-7000-8000-000000000001";

const RECORDS: components["schemas"]["MagicLineRecords"] = {
  data: [
    {
      audit_id: "00000000-0000-7000-8000-0000000000a1",
      occurred_at: "2026-09-13T07:30:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c1",
        label: "GEM",
      },
      changes: [{ field: "industry", after: "Software" }],
      undo: {
        undoable: true,
        audit_id: "00000000-0000-7000-8000-0000000000a1",
        version: 3,
      },
    },
    {
      audit_id: "00000000-0000-7000-8000-0000000000a2",
      occurred_at: "2026-09-13T07:29:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c2",
        label: "Kandu GmbH",
      },
      changes: [{ field: "industry", before: "Retail", after: "Software" }],
      undo: { undoable: false, reason: "already_undone" },
    },
  ],
  page: { has_more: false, total: 2 },
};

function stubOpenedLine(records = RECORDS, activityStatus = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const path = url.split("?")[0];
      if (path.endsWith("/activities/email-1"))
        return activityStatus === 200
          ? jsonResponse({
              id: "email-1",
              kind: "email",
              subject: "Planning request",
            })
          : jsonResponse(
              { code: "not_found", title: "Not found", status: 404 },
              404,
            );
      if (path.endsWith("/restore")) return jsonResponse({ id: "restore-1" });

      if (path.endsWith(`/magic/lines/${LINE}/records`)) {
        return jsonResponse(records);
      }
      if (path.endsWith("/magic")) {
        return jsonResponse(
          receipt({
            done: [
              line({
                summary: {
                  key: "magic.action.fields_changed",
                  values: { fields: "industry" },
                },
                entity: {
                  type: "company",
                  id: "00000000-0000-7000-8000-0000000000c1",
                  label: "GEM",
                },
                count: 2,
                undo: undefined,
              }),
            ],
            totals: {
              done: 1,
              needs_you: 0,
              could_not_complete: 0,
              watching: 0,
            },
          }),
        );
      }
      return jsonResponse({ data: [] });
    }),
  );
}

describe("a done line standing for many records", () => {
  it("opens to each record, what changed on it, and its own undo", async () => {
    stubOpenedLine();
    renderMagic();

    await userEvent.click(
      await screen.findByRole("button", { name: "GEM and 1 more" }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Kandu GmbH")).toBeTruthy();
    expect(within(dialog).getByText("empty → Software")).toBeTruthy();
    expect(within(dialog).getByText("Retail → Software")).toBeTruthy();
    // One press per record: the undoable one offers it, the other says why not.
    expect(
      within(dialog).getAllByRole("button", { name: "Undo" }),
    ).toHaveLength(1);
    expect(
      within(dialog).getByText("This change was already undone."),
    ).toBeTruthy();

    await waitFor(() => {
      const opened = vi
        .mocked(fetch)
        .mock.calls.map(([input]) => String((input as Request).url))
        .find((url) => url.includes(`/magic/lines/${LINE}/records`));
      expect(opened).toContain("since=");
    });
  });
});

it("keeps the complete changed value available beside its record link", async () => {
  const value =
    "A detailed service description. ".repeat(12) +
    "Final supported capability.";
  stubOpenedLine({
    ...RECORDS,
    data: RECORDS.data.slice(0, 1).map((record) => ({
      ...record,
      changes: [{ field: "industry", after: value }],
    })),
  });
  renderMagic();
  await userEvent.click(
    await screen.findByRole("button", { name: "GEM and 1 more" }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText(`empty → ${value}`)).toBeTruthy();
  expect(
    within(dialog).getByRole("link", { name: "GEM" }).getAttribute("href"),
  ).toContain("companies");
});

it("keeps separate changes to the same technical field and uses its shared label", () => {
  render(
    <StoryProviders>
      <ChangeList
        changes={[
          { field: "operated_service", before: "Old shop", after: "New shop" },
          {
            field: "operated_service",
            before: "Old portal",
            after: "New portal",
          },
        ]}
      />
    </StoryProviders>,
  );
  expect(screen.getByText("Old shop → New shop")).toBeTruthy();
  expect(screen.getByText("Old portal → New portal")).toBeTruthy();
  expect(screen.getAllByText("Operated service")).toHaveLength(2);
});

function activityRecords(): components["schemas"]["MagicLineRecords"] {
  return {
    data: [
      {
        audit_id: "audit-1",
        occurred_at: "2026-09-13T07:30:00Z",
        entity: { type: "activity", id: "email-1", label: "Planning request" },
        changes: [],
        undo: { undoable: true, audit_id: "audit-1", version: 4 },
      },
    ],
    page: { has_more: false },
  };
}

it("opens a receipt email in its full reader after closing the receipt", async () => {
  stubOpenedLine(activityRecords());
  renderMagic();
  await userEvent.click(
    await screen.findByRole("button", { name: "GEM and 1 more" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Planning request" }),
  );
  expect(window.location.hash).toContain("Planning%20request/email-1");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("keeps the receipt and explains a refused source read", async () => {
  stubOpenedLine(activityRecords(), 404);
  renderMagic();
  await userEvent.click(
    await screen.findByRole("button", { name: "GEM and 1 more" }),
  );
  const hash = window.location.hash;
  await userEvent.click(
    await screen.findByRole("button", { name: "Planning request" }),
  );
  expect(
    await within(screen.getByRole("dialog")).findByRole("status"),
  ).toBeTruthy();
  expect(window.location.hash).toBe(hash);
});

it("restores an activity through the versioned history writer", async () => {
  stubOpenedLine(activityRecords());
  renderMagic();
  await userEvent.click(
    await screen.findByRole("button", { name: "GEM and 1 more" }),
  );
  await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
  await waitFor(() => {
    const request = vi
      .mocked(fetch)
      .mock.calls.map(([input]) => input)
      .find(
        (input) =>
          input instanceof Request &&
          input.url.endsWith(
            "/records/activity/email-1/history/audit-1/restore",
          ),
      );
    expect(request).toBeInstanceOf(Request);
    if (!(request instanceof Request))
      throw new Error("Restore request missing");
    expect(request.method).toBe("POST");
    expect(request.headers.get("If-Match")).toContain("4");
  });
});

it("formats recorded money and resolves users without exposing opaque identifiers", async () => {
  const user = "11111111-1111-4111-8111-111111111111";
  const unknown = "22222222-2222-4222-8222-222222222222";
  installFetchStub({
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: user, display_name: "Morgan Reed" }] }),
  });
  const view = render(
    <StoryProviders>
      <ChangeList
        changes={[
          { field: "owner_id", before: unknown, after: user },
          { field: "currency", before: "EUR", after: "EUR" },
          { field: "amount_minor", before: 2500000, after: 4150000 },
        ]}
      />
    </StoryProviders>,
  );
  expect(
    await screen.findByText("Name unavailable → Morgan Reed"),
  ).toBeTruthy();
  expect(screen.getByText(/25,000.*41,500/)).toBeTruthy();
  expect(view.container.textContent).not.toContain(user);
  expect(view.container.textContent).not.toContain(unknown);
  expect(view.container.querySelector(`[title="${unknown}"]`)).toBeNull();
});

it("does not invent a currency for amounts or show unknown record ids", () => {
  const id = "22222222-2222-4222-8222-222222222222";
  const view = render(
    <StoryProviders>
      <ChangeList
        changes={[
          { field: "amount_minor", before: 2500000, after: 4150000 },
          { field: "company_id", after: id },
        ]}
      />
    </StoryProviders>,
  );
  expect(
    screen.getByText(
      "2,500,000 minor units (currency not recorded) → 4,150,000 minor units (currency not recorded)",
    ),
  ).toBeTruthy();
  expect(screen.getByText("empty → Name unavailable")).toBeTruthy();
  expect(view.container.textContent).not.toContain(id);
});

it("describes an archive as an action and date rather than an empty-field edit", () => {
  render(
    <StoryProviders>
      <ChangeList
        changes={[{ field: "archived_at", after: "2026-10-20T08:30:00Z" }]}
      />
    </StoryProviders>,
  );
  expect(screen.getByText("Archived")).toBeTruthy();
  expect(screen.queryByText(/empty →/)).toBeNull();
  expect(screen.queryByText("archived at")).toBeNull();
});
