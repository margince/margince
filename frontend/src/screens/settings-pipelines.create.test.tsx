/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../testing/mefixture";
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
function server(
  start: readonly Pipeline[],
  me: Readonly<{ allow?: GrantSpec; seat?: "full" | "read" }> = {},
) {
  let catalog = [...start];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request =
        input instanceof Request ? input : new Request(String(input));
      const path = new URL(request.url).pathname;
      if (path.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({ allow: me.allow ?? PIPELINE_ADMIN, seat: me.seat }),
        );
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

// A refused button is disabled and points at its reason, which a reader sees.
function expectRefused(button: HTMLElement, reason: string) {
  expect((button as HTMLButtonElement).disabled).toBe(true);
  const describedBy = button.getAttribute("aria-describedby") ?? "";
  expect(document.getElementById(describedBy)?.textContent).toBe(reason);
}

const TWO_PIPELINES = [
  SALES,
  { ...SALES, id: "pl-2", name: "Enterprise", is_default: false },
];

describe("PipelinesCard: a refusal that lands mid-edit", () => {
  it("closes the New pipeline dialog when the create grant is withdrawn", async () => {
    const user = userEvent.setup();
    server(TWO_PIPELINES);
    const { client } = render(<PipelinesCard />);
    await user.click(
      await screen.findByRole("button", { name: "New pipeline" }),
    );
    expect(screen.getByLabelText(/Name/)).toBeTruthy();
    act(() => {
      client.setQueryData(
        ["me"],
        meFixture({ allow: { pipeline: ["read", "update"] } }),
      );
    });
    await waitFor(() => expect(screen.queryByLabelText(/Name/)).toBeNull());
    expectRefused(
      screen.getByRole("button", { name: /New pipeline/ }),
      "Your role cannot create pipelines. Ask an admin.",
    );
  });

  it("closes the New stage dialog when the update grant is withdrawn", async () => {
    const user = userEvent.setup();
    server(TWO_PIPELINES);
    const { client } = render(<PipelinesCard />);
    await user.click(await screen.findByTestId("new-stage-pl"));
    expect(screen.getByLabelText(/Name/)).toBeTruthy();
    act(() => {
      client.setQueryData(
        ["me"],
        meFixture({ allow: { pipeline: ["read", "create"] } }),
      );
    });
    await waitFor(() => expect(screen.queryByLabelText(/Name/)).toBeNull());
    expectRefused(
      screen.getByTestId("new-stage-pl"),
      "Your role cannot add stages. Ask an admin.",
    );
  });
});

describe("PipelinesCard: a read seat", () => {
  // The role grants both verbs and the seat is what refuses. So the sentence
  // must send the reader to a seat upgrade rather than a role change.
  it("refuses the create verbs for the seat, not the role", async () => {
    server(TWO_PIPELINES, { seat: "read" });
    render(<PipelinesCard />);
    const seat = "Your seat is read-only. Ask an admin for a full seat.";
    expectRefused(
      await screen.findByRole("button", { name: /New pipeline/ }),
      seat,
    );
    expectRefused(screen.getByTestId("new-stage-pl"), seat);
  });
});
