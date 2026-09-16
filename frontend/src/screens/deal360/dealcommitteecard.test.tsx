/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { meFixture } from "../../app/mefixture";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { DealCommitteeCard } from "./dealcommitteecard";

// The card joins two reads of ONE set of edges: the stakeholder rows carry the
// verbs, the coverage read carries who has answered. What is asserted here is
// the join — that engagement lands on the row for the same contact, and that a
// row the coverage read does not cover claims nothing about it. A card that
// guessed there would report a silence nobody measured.

type DealCoverage = components["schemas"]["DealCoverage"];

const DEAL = "01a02e25-a5ac-7099-8099-581cbf001a99";
const TALKING = "01a02be9-2293-75d2-9dd2-3027d9b63dc2";
const QUIET = "01a02be9-4471-7a10-8a10-6f2b1cc4e0d1";
const UNCOVERED = "01a02be9-9c02-7bb4-8bb4-11e4a7d3f550";

const NAMES: Record<string, string> = {
  [TALKING]: "Mai Trần",
  [QUIET]: "Bảo Nguyễn",
  [UNCOVERED]: "Linh Phạm",
};

function edge(contactId: string, role: string) {
  return {
    id: `rel-${contactId}`,
    kind: "deal_stakeholder",
    deal_id: DEAL,
    contact_id: contactId,
    role,
    is_current_primary: false,
    source: "manual",
    captured_by: "human:u-1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

type Patch = { role: unknown; ifMatch: string | null };

function stubFetch(
  edges: unknown[],
  deleted: string[] = [],
  patched: Patch[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { url, method } = request;
      if (method === "DELETE") {
        deleted.push(url.slice(url.lastIndexOf("/") + 1));
        return json({}, 204);
      }
      if (method === "PATCH") {
        const body = await request.json();
        patched.push({
          role: body.role,
          ifMatch: request.headers.get("If-Match"),
        });
        return json({ ...edge(TALKING, String(body.role)), version: 2 });
      }
      if (url.endsWith("/v1/me")) {
        return json(
          meFixture({
            allow: { relationship: ["read", "create", "update", "delete"] },
          }),
        );
      }
      if (url.includes("/relationships")) {
        return json({ data: edges, page: { next_cursor: null } });
      }
      // EntityRef resolves each far end by id, which is what puts a name on a
      // row rather than the id the edge carries.
      const named = Object.keys(NAMES).find((id) => url.includes(id));
      if (named) {
        return json({ id: named, full_name: NAMES[named] });
      }
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
}

const coverage = (seats: DealCoverage["stakeholders"]): DealCoverage => ({
  deal_id: DEAL,
  stakeholders: seats,
  our_side: [],
  risks: [],
  sections_omitted: [],
});

function draw(
  edges: unknown[],
  view?: DealCoverage,
  deleted: string[] = [],
  patched: Patch[] = [],
) {
  stubFetch(edges, deleted, patched);
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <DealCommitteeCard
          dealId={DEAL}
          coverage={view}
          withheld={false}
          pending={false}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/** The block a seat's facts are read from, found by the contact's own name. */
async function seatOf(name: string) {
  const link = await screen.findByRole("link", { name });
  const block = link.closest(".panel-row");
  if (!block) {
    throw new Error(`the seat for ${name} is not drawn as a row of its own`);
  }
  return within(block as HTMLElement);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the deal's committee card", () => {
  it("puts each seat's engagement on that seat's own row", async () => {
    draw(
      [edge(TALKING, "economic_buyer"), edge(QUIET, "evaluator")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
        { contact_id: QUIET, role: "evaluator", engaged: false },
      ]),
    );

    expect(
      (await seatOf("Mai Trần")).getByText(en["coverage.engaged"]),
    ).toBeInTheDocument();
    expect(
      (await seatOf("Bảo Nguyễn")).getByText(en["coverage.quiet"]),
    ).toBeInTheDocument();
  });

  it("claims nothing about a seat the coverage read did not cover", async () => {
    // The two reads can disagree — a seat added since the coverage view was
    // built, or a view the reader is refused. Printing "quiet" there would
    // report a silence nobody measured, which is the one wrong answer
    // available: it reads exactly like a stakeholder who has not replied.
    draw(
      [edge(TALKING, "economic_buyer"), edge(UNCOVERED, "user")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
      ]),
    );

    const uncovered = await seatOf("Linh Phạm");
    expect(uncovered.queryByText(en["coverage.quiet"])).toBeNull();
    expect(uncovered.queryByText(en["coverage.engaged"])).toBeNull();
    // The seat is still a seat: the row and its other facts stand.
    expect(uncovered.getByText("user")).toBeInTheDocument();
  });

  // Remove is a hard DELETE with no restore path, so the card asks first. The
  // three cases below are the three answers a reader can give it, and the two
  // that must send nothing are the point: a seat taken off a deal by a stray
  // click is a stakeholder the next reader does not know was ever there.
  it("asks before it takes a seat off the deal, and sends nothing yet", async () => {
    const deleted: string[] = [];
    draw(
      [edge(TALKING, "economic_buyer")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
      ]),
      deleted,
    );

    await userEvent.click(await screen.findByTestId("remove-relationship"));

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(deleted).toEqual([]);
  });

  it("takes the seat off once the reader confirms it", async () => {
    const deleted: string[] = [];
    draw(
      [edge(TALKING, "economic_buyer")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
      ]),
      deleted,
    );

    await userEvent.click(await screen.findByTestId("remove-relationship"));
    await userEvent.click(
      await screen.findByTestId("remove-relationship-confirm"),
    );

    expect(deleted).toEqual([`rel-${TALKING}`]);
  });

  it("leaves the seat where it is when the reader backs out", async () => {
    const deleted: string[] = [];
    draw(
      [edge(TALKING, "economic_buyer")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
      ]),
      deleted,
    );

    await userEvent.click(await screen.findByTestId("remove-relationship"));
    await userEvent.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: en["create.cancel"],
      }),
    );

    expect(deleted).toEqual([]);
    expect(await screen.findByRole("link", { name: "Mai Trần" })).toBeVisible();
  });

  it("offers no way to add a seat to a reader whose role holds no grant", async () => {
    // The verb is WITHHELD rather than refused: a reader with no
    // relationship:create grant is told nothing about this deal by its absence,
    // where a refused control would name a rule that is not about the record.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.endsWith("/v1/me")) {
          return json(meFixture({ allow: { relationship: ["read"] } }));
        }
        if (request.url.includes("/relationships")) {
          return json({ data: [], page: { next_cursor: null } });
        }
        return json({ data: [], page: { next_cursor: null } });
      }),
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <DealCommitteeCard
            dealId={DEAL}
            coverage={coverage([])}
            withheld={false}
            pending={false}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(en["rel.dealStakeholdersEmpty"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["rel.addStakeholder"] }),
    ).toBeNull();
  });

  it("sends a changed role against the row it was read from", async () => {
    // The edit PATCHes with If-Match carrying the version the row was drawn
    // with, so a seat somebody else moved in the meantime refuses the write
    // rather than quietly overwriting their answer with a stale one.
    const patched: Patch[] = [];
    draw(
      [edge(TALKING, "economic_buyer")],
      coverage([
        { contact_id: TALKING, role: "economic_buyer", engaged: true },
      ]),
      [],
      patched,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: en["record.edit"] }),
    );
    const form = within(await screen.findByRole("dialog"));
    const role = form.getByLabelText(en["rel.role"]);
    await userEvent.clear(role);
    await userEvent.type(role, "champion");
    await userEvent.click(
      form.getByRole("button", { name: en["record.save"] }),
    );

    expect(patched).toEqual([{ role: "champion", ifMatch: "1" }]);
  });

  it("says the committee is empty once, not twice", async () => {
    // The picture and the rows both know the committee is empty, and both used
    // to say so. The rows keep the sentence, because they carry the verb that
    // ends the emptiness.
    draw([], coverage([]));

    expect(
      await screen.findByText(en["rel.dealStakeholdersEmpty"]),
    ).toBeInTheDocument();
    expect(screen.getAllByText(en["rel.dealStakeholdersEmpty"])).toHaveLength(
      1,
    );
  });
});
