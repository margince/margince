/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { suggestion } from "../../testing/dealsuggestion.fixture";
import { meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { HomeRepairControls } from "./homerepair";
import {
  classificationRepairReview,
  proposalRepairReview as proposalReview,
  provenanceRepairReview,
  repairReview as review,
  settlementRepairReview,
  suggestionRepairReview,
} from "./homerepair.fixtures";

function serve(
  status = 204,
  payload = review,
  previews: unknown[] = [],
  mayRetire = true,
  mayReclassify = true,
  mayArchive = true,
) {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (request.url.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            allow: {
              deal: mayRetire ? ["read", "create"] : ["read"],
              activity: mayArchive
                ? mayReclassify
                  ? ["read", "update", "delete"]
                  : ["read", "delete"]
                : mayReclassify
                  ? ["read", "update"]
                  : ["read"],
            },
          }),
        );
      }
      if (request.url.includes("/deal-suggestions/")) {
        return jsonResponse(suggestion);
      }
      if (request.url.endsWith("/preview")) {
        previews.push(await request.json());
        return jsonResponse(payload);
      }
      if (request.url.endsWith("/activate")) {
        writes.push(await request.json());
        return status === 204
          ? new Response(null, { status })
          : jsonResponse(
              { code: "conflict", message: "Review changed." },
              status,
            );
      }
      return jsonResponse({
        subject: "Prepare account review",
        summary: "Prepare account review",
      });
    }),
  );
  return writes;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the exact deal suggestion before activating its reviewed retirement", async () => {
  const previews: unknown[] = [];
  const writes = serve(204, suggestionRepairReview, previews);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.suggestions"] }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  expect(previews).toEqual([{ limit: 25, source: "suggestion" }]);
  expect(
    screen.getByText(englishCopy["homeRepair.supersede"]),
  ).toBeInTheDocument();
  const inspect = screen.getByRole("button", { name: "Inspect" });
  await waitFor(() => expect(inspect).toBeEnabled());
  fireEvent.click(inspect);
  expect(await screen.findByRole("dialog")).toBeInTheDocument();
  expect(
    await screen.findByText(
      /The buyer requested a platform migration proposal/,
    ),
  ).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  fireEvent.click(apply);
  await waitFor(() =>
    expect(writes).toEqual([{ version: 7, review: suggestionRepairReview }]),
  );
});

it("allows suggestion inspection but explains missing retirement authority", async () => {
  const writes = serve(204, suggestionRepairReview, [], false);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.suggestions"] }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(englishCopy["homeRepair.suggestionPermission"]),
  ).toBeInTheDocument();
  const inspect = screen.getByRole("button", { name: "Inspect" });
  await waitFor(() => expect(inspect).toBeEnabled());
  expect(
    screen.queryByRole("button", { name: "Apply 1 reviewed change" }),
  ).toBeNull();
  expect(writes).toEqual([]);
});

it("requires a separate explicit action after showing named tasks and their dispositions", async () => {
  const writes = serve();
  render(<HomeRepairControls version={7} canActivate />);
  expect(screen.queryByRole("button", { name: /Apply 1 reviewed/ })).toBeNull();
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  await waitFor(() =>
    expect(screen.getAllByText("Prepare account review")).toHaveLength(2),
  );
  expect(
    screen.getByText(englishCopy["homeRepair.keepReview"]),
  ).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(apply);
  await waitFor(() => expect(writes).toEqual([{ version: 7, review }]));
  expect(
    await screen.findByText(englishCopy["homeRepair.started"]),
  ).toBeInTheDocument();
});
it("allows inspection without activation rights", async () => {
  serve();
  render(<HomeRepairControls version={7} canActivate={false} />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  await screen.findByText(englishCopy["homeRepair.keepReview"]);
  expect(screen.queryByRole("button", { name: /Apply 1 reviewed/ })).toBeNull();
});
it("keeps a refused review visible and never reports it queued", async () => {
  serve(409);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "Apply 1 reviewed change" }),
  );
  await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
  expect(
    screen.getByText(englishCopy["homeRepair.keepReview"]),
  ).toBeInTheDocument();
  expect(screen.queryByText(englishCopy["homeRepair.started"])).toBeNull();
});

it("reviews proposals separately and activates only their displayed snapshot", async () => {
  const previews: unknown[] = [];
  const writes = serve(204, proposalReview, previews);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(screen.getByRole("button", { name: "Proposals" }));
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  expect(previews).toEqual([{ limit: 25, source: "approval" }]);
  expect(
    screen.getByText(englishCopy["homeRepair.withdraw"]),
  ).toBeInTheDocument();
  expect(
    screen.getByText(englishCopy["homeRepair.reason.duplicate"]),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "Results to change: 0. Automatic tasks to archive: 0. Proposals to withdraw: 1. Deal suggestions to retire: 0. Keep for review: 0; keep as current work: 0.",
    ),
  ).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(apply);
  await waitFor(() =>
    expect(writes).toEqual([{ version: 7, review: proposalReview }]),
  );
});

it("shows the exact classification change and submits its server review token", async () => {
  const previews: unknown[] = [];
  const writes = serve(204, classificationRepairReview, previews);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", {
      name: englishCopy["homeRepair.classifications"],
    }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(
      "Current: Requests action. Proposed: Information only.",
    ),
  ).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Apply 1 reviewed change" }),
  );
  await waitFor(() =>
    expect(writes).toEqual([
      { version: 7, review: classificationRepairReview },
    ]),
  );
  expect(previews).toEqual([{ limit: 25, source: "verdict" }]);
});

it("preserves read-only classification review without offering a refused write", async () => {
  serve(204, classificationRepairReview, [], true, false);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", {
      name: englishCopy["homeRepair.classifications"],
    }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(englishCopy["homeRepair.classificationPermission"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Apply 1 reviewed change" }),
  ).not.toBeInTheDocument();
});

it("keeps task review readable without offering an unauthorized archive", async () => {
  serve(204, review, [], true, true, false);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(englishCopy["homeRepair.taskPermission"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /Apply .* reviewed change/ }),
  ).not.toBeInTheDocument();
});

it("reviews older answered requests and activates only their revalidation", async () => {
  const previews: unknown[] = [];
  const writes = serve(204, settlementRepairReview, previews);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.settlements"] }),
  );
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText("1 request is judged again."),
  ).toBeInTheDocument();
  expect(
    screen.getByText(englishCopy["homeRepair.reason.settlementHuman"]),
  ).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Apply 1 reviewed change" }),
  );
  await waitFor(() =>
    expect(writes).toEqual([{ version: 7, review: settlementRepairReview }]),
  );
  expect(previews).toEqual([{ limit: 25, source: "settlement" }]);
});

it("reviews source and date corrections with update authority and no archive authority", async () => {
  const writes = serve(204, provenanceRepairReview, [], true, true, false);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(englishCopy["homeRepair.normalize"]),
  ).toBeInTheDocument();
  expect(
    screen.getByText(englishCopy["homeRepair.reason.restoreSourceAndDate"]),
  ).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Apply 1 reviewed change" }),
  );
  await waitFor(() =>
    expect(writes).toEqual([
      {
        version: 7,
        review: provenanceRepairReview,
        reviewed_actions: ["normalize_automatic_task"],
      },
    ]),
  );
});

it("keeps source corrections read-only without update authority", async () => {
  const writes = serve(204, provenanceRepairReview, [], true, false, true);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(
    screen.getByRole("button", { name: englishCopy["homeRepair.preview"] }),
  );
  expect(
    await screen.findByText(englishCopy["homeRepair.normalize"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Apply 1 reviewed change" }),
  ).toBeNull();
  expect(writes).toEqual([]);
});
