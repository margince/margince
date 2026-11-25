// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { ConfirmAdvanceModal, type PendingAdvance } from "./confirmadvance";
import { askedToClose } from "./confirmadvance.testkit";

// Winning a deal moves its project into delivering when the project is live
// and still in initiative or pursuing. The won dialog says so before it happens.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const WON: components["schemas"]["Stage"] = {
  id: "stage-won",
  pipeline_id: "pl-1",
  name: "Closed won",
  position: 4,
  semantic: "won",
  win_probability: 100,
};

const LOST: components["schemas"]["Stage"] = {
  ...WON,
  id: "stage-lost",
  name: "Closed lost",
  semantic: "lost",
  win_probability: 0,
};

type ProjectRead =
  | Readonly<{
      phase: string;
      archived_at?: string | null;
    }>
  | "unreadable"
  | null;

// The deal names project pr-1 unless `project` is null; every other read
// answers an empty page. `answer` swaps what later reads see.
function stubReads(initial: ProjectRead) {
  const reads: string[] = [];
  let project = initial;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      reads.push(url);
      const path = new URL(url, "http://test").pathname;
      if (path.endsWith("/deals/deal-a")) {
        return Response.json({
          id: "deal-a",
          name: "Wendt renewal",
          project_id: project ? "pr-1" : null,
        });
      }
      if (path.endsWith("/projects/pr-1") && project === "unreadable") {
        return Response.json(
          { status: 404, title: "Not found" },
          {
            status: 404,
            headers: { "Content-Type": "application/problem+json" },
          },
        );
      }
      if (
        path.endsWith("/projects/pr-1") &&
        typeof project === "object" &&
        project
      ) {
        return Response.json({ id: "pr-1", name: "Depot fit-out", ...project });
      }
      if (path.endsWith("/win-evidence")) {
        return Response.json({ has_signed_contract: true });
      }
      return Response.json({ data: [], page: { has_more: false } });
    }),
  );
  return {
    reads,
    answer: (next: ProjectRead) => {
      project = next;
    },
  };
}

function openOn(toStage: components["schemas"]["Stage"]) {
  const client = new QueryClient();
  const shown = (pending: PendingAdvance | null) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmAdvanceModal
          pending={pending}
          onClose={() => {}}
          onConfirm={async () => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(shown({ dealId: "deal-a", version: 3, toStage }));
  return {
    close: () => rerender(shown(null)),
    reopen: () => rerender(shown({ dealId: "deal-a", version: 3, toStage })),
  };
}

it("says the win also moves a pursuing project to Delivering", async () => {
  stubReads({ phase: "pursuing" });
  openOn(WON);
  expect(
    await screen.findByText("This also moves Depot fit-out to Delivering."),
  ).toBeTruthy();
});

it("says it for a project still in initiative too", async () => {
  stubReads({ phase: "initiative" });
  openOn(WON);
  expect(
    await screen.findByText("This also moves Depot fit-out to Delivering."),
  ).toBeTruthy();
});

it.each([
  ["already delivering", { phase: "delivering" }],
  ["closed", { phase: "closed" }],
  ["archived", { phase: "pursuing", archived_at: "2026-10-01T08:00:00Z" }],
])("says nothing of a project the win leaves alone: %s", async (_, project) => {
  const { reads } = stubReads(project);
  openOn(WON);
  await askedToClose("Closed won");
  await waitFor(() =>
    expect(reads.some((url) => url.includes("/projects/pr-1"))).toBe(true),
  );
  expect(screen.queryByTestId("win-moves-project")).toBeNull();
});

it("says nothing when the deal has no project", async () => {
  stubReads(null);
  openOn(WON);
  await askedToClose("Closed won");
  expect(screen.queryByTestId("win-moves-project")).toBeNull();
});

it("reads no project for a loss, which moves none", async () => {
  const { reads } = stubReads({ phase: "pursuing" });
  openOn(LOST);
  await askedToClose("Closed lost");
  expect(reads.some((url) => url.includes("/projects/"))).toBe(false);
  expect(screen.queryByTestId("win-moves-project")).toBeNull();
});

it("says the win moves its project when the reader may not read that project", async () => {
  stubReads("unreadable");
  openOn(WON);
  expect(
    await screen.findByText(
      en["deals.confirmMovesUnreadProject"].replace(
        "{phase}",
        en["project.phase.delivering"],
      ),
    ),
  ).toBeTruthy();
});

it("reads the deal again at each opening, and names a project attached since", async () => {
  const deal = stubReads(null);
  const dialog = openOn(WON);
  await askedToClose("Closed won");
  expect(screen.queryByTestId("win-moves-project")).toBeNull();

  dialog.close();
  deal.answer({ phase: "pursuing" });
  dialog.reopen();
  expect(
    await screen.findByText("This also moves Depot fit-out to Delivering."),
  ).toBeTruthy();
});

it("drops the sentence at the next opening once the project has moved on", async () => {
  const deal = stubReads({ phase: "pursuing" });
  const dialog = openOn(WON);
  await screen.findByText("This also moves Depot fit-out to Delivering.");

  dialog.close();
  deal.answer({ phase: "delivering" });
  const readsBefore = deal.reads.length;
  dialog.reopen();
  expect(screen.queryByTestId("win-moves-project")).toBeNull();
  await waitFor(() =>
    expect(
      deal.reads
        .slice(readsBefore)
        .some((url) => url.includes("/projects/pr-1")),
    ).toBe(true),
  );
  expect(screen.queryByTestId("win-moves-project")).toBeNull();
});
