/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { suggestion } from "../deals/dealsuggestion.fixture";
import { HomeRepairControls } from "./homerepair";
import {
  proposalRepairReview as proposalReview,
  repairReview as review,
  suggestionRepairReview,
} from "./homerepair.fixtures";
import { jsonResponse, render } from "./settings.testkit";

function serve(
  status = 204,
  payload = review,
  previews: unknown[] = [],
  mayRetire = true,
) {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (request.url.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            allow: { deal: mayRetire ? ["read", "create"] : ["read"] },
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
  fireEvent.click(screen.getByRole("button", { name: "Deal suggestions" }));
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  expect(previews).toEqual([{ limit: 25, source: "suggestion" }]);
  expect(
    screen.getByText("Retire automatic deal suggestion"),
  ).toBeInTheDocument();
  const inspect = screen.getByRole("button", { name: "Inspect" });
  await waitFor(() => expect(inspect).toBeEnabled());
  fireEvent.click(inspect);
  expect(await screen.findByRole("dialog")).toBeInTheDocument();
  expect(
    screen.getByText(/The buyer requested a platform migration proposal/),
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
  fireEvent.click(screen.getByRole("button", { name: "Deal suggestions" }));
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  expect(
    await screen.findByText(
      "Deal creation permission is required to retire these suggestions.",
    ),
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
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  await waitFor(() =>
    expect(screen.getAllByText("Prepare account review")).toHaveLength(2),
  );
  expect(screen.getByText("Keep for human review")).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(apply);
  await waitFor(() => expect(writes).toEqual([{ version: 7, review }]));
  expect(
    await screen.findByText(
      "The reviewed run is queued. Its progress appears above.",
    ),
  ).toBeInTheDocument();
});
it("allows inspection without activation rights", async () => {
  serve();
  render(<HomeRepairControls version={7} canActivate={false} />);
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  await screen.findByText("Keep for human review");
  expect(screen.queryByRole("button", { name: /Apply 1 reviewed/ })).toBeNull();
});
it("keeps a refused review visible and never reports it queued", async () => {
  serve(409);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Apply 1 reviewed change" }),
  );
  await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
  expect(screen.getByText("Keep for human review")).toBeInTheDocument();
  expect(
    screen.queryByText(
      "The reviewed run is queued. Its progress appears above.",
    ),
  ).toBeNull();
});

it("reviews proposals separately and activates only their displayed snapshot", async () => {
  const previews: unknown[] = [];
  const writes = serve(204, proposalReview, previews);
  render(<HomeRepairControls version={7} canActivate />);
  fireEvent.click(screen.getByRole("button", { name: "Proposals" }));
  fireEvent.click(screen.getByRole("button", { name: "Preview items" }));
  const apply = await screen.findByRole("button", {
    name: "Apply 1 reviewed change",
  });
  expect(previews).toEqual([{ limit: 25, source: "approval" }]);
  expect(screen.getByText("Withdraw automatic proposal")).toBeInTheDocument();
  expect(
    screen.getByText("An identical pending proposal remains available."),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "Reminders to archive: 0. Proposals to withdraw: 1. Deal suggestions to retire: 0. Keep for review: 0; keep as current work: 0.",
    ),
  ).toBeInTheDocument();
  expect(writes).toEqual([]);
  fireEvent.click(apply);
  await waitFor(() =>
    expect(writes).toEqual([{ version: 7, review: proposalReview }]),
  );
});
