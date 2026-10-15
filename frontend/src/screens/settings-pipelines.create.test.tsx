/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../testing/mefixture";
import { PipelinesCard } from "./settings.pipelines";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The second pipeline brings the catalog, which takes "New pipeline" off the
// lone pipeline's head. Focus must not fall to <body>.

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/settings/pipelines");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Pipeline = Readonly<{
  id: string;
  name: string;
  is_default: boolean;
  position: number;
  version: number;
  archived_at?: string;
  stages: readonly unknown[];
}>;

const SALES: Pipeline = {
  id: "pl",
  name: "Sales",
  is_default: true,
  position: 0,
  version: 3,
  stages: [],
};

const RETIRED: Pipeline = {
  id: "pl-old",
  name: "Retired line",
  is_default: false,
  position: 1,
  version: 4,
  archived_at: "2026-09-01T00:00:00Z",
  stages: [],
};

// The catalog is state: a created pipeline is in the next read.
function server(start: readonly Pipeline[]) {
  let catalog = [...start];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request =
        input instanceof Request ? input : new Request(String(input));
      const path = new URL(request.url).pathname;
      if (path.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: PIPELINE_ADMIN }));
      }
      if (path.endsWith("/v1/pipelines") && request.method === "POST") {
        const body = await request.json();
        const created = { id: "pl-new", version: 1, ...body };
        catalog = [...catalog, created];
        return jsonResponse(created, 201);
      }
      if (path.endsWith("/v1/pipelines")) {
        return jsonResponse({ data: catalog, page: { next_cursor: null } });
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
}

async function createFromHead(alone: string) {
  const user = userEvent.setup();
  render(<PipelinesCard />);
  const head = await screen.findByRole("region", { name: alone });
  await user.click(
    await within(head).findByRole("button", { name: "New pipeline" }),
  );
  await user.type(screen.getByLabelText(/Name/), "Partnerships");
  await user.click(screen.getByRole("button", { name: "Create" }));
  await screen.findByRole("region", { name: "All pipelines" });
}

describe("PipelinesCard: the second pipeline", () => {
  it("hands focus to the new pipeline's name when one in use becomes two", async () => {
    server([SALES]);
    await createFromHead("Sales");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { name: "Partnerships" }).firstElementChild,
      ),
    );
  });

  it("hands focus to the new pipeline's name when only a retired one stood", async () => {
    server([RETIRED]);
    await createFromHead("Retired line");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { name: "Partnerships" }).firstElementChild,
      ),
    );
  });

  it("leaves focus on New pipeline when the catalog already held it", async () => {
    server([
      SALES,
      { ...SALES, id: "pl-2", name: "Enterprise", is_default: false },
    ]);
    await createFromHead("All pipelines");
    await screen.findByRole("heading", { name: "Partnerships" });
    expect(document.activeElement?.textContent).toMatch(/New pipeline/);
  });
});
