/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { PipelinesCard } from "./settings.pipelines";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The pipelines page: the catalog above the pipeline it opens, and that
// pipeline's ladder. Each affordance follows its OWN verb (create, update,
// delete), so the cases below grant one at a time.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  globalThis.history.replaceState(null, "", "#/settings/pipelines");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

type Call = Readonly<{
  method: string;
  url: string;
  ifMatch: string | null;
  body: unknown;
}>;

function stage(
  id: string,
  name: string,
  position: number,
  semantic: string,
  win_probability: number,
) {
  return { id, pipeline_id: "pl", name, position, semantic, win_probability };
}

// The server, as far as this page talks to it: the catalog and the Sales
// ladder are STATE, so a saved order is what the next read answers — which is
// what an Undo drawn from the latest version has to be tested against.
function server(opts: {
  roles?: string[];
  allow?: GrantSpec;
  stageOrderRefusal?: { status: number; body: unknown };
  stageDeleteRefusal?: { status: number; body: unknown };
}) {
  const calls: Call[] = [];
  let salesVersion = 3;
  let sales = [
    stage("s1", "Qualify", 1, "open", 20),
    stage("s1b", "Proposal", 2, "open", 50),
    stage("s2", "Closed won", 3, "won", 100),
    stage("s3", "Closed lost", 4, "lost", 0),
  ];
  const salesPipeline = () => ({
    id: "pl",
    name: "Sales",
    is_default: true,
    position: 0,
    version: salesVersion,
    stages: sales,
  });
  const catalog = () => [
    {
      id: "pl-old",
      name: "Retired line",
      is_default: false,
      position: 1,
      version: 4,
      archived_at: "2026-09-01T00:00:00Z",
      stages: [],
    },
    // Live and NOT the default: the only shape that can actually be retired.
    {
      id: "pl-live",
      name: "Enterprise",
      is_default: false,
      position: 2,
      version: 7,
      stages: [],
    },
    salesPipeline(),
  ];
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    const url = request.url;
    const method = request.method;
    const raw = await request.clone().text();
    const call = {
      method,
      url,
      ifMatch: request.headers.get("If-Match"),
      body: raw ? JSON.parse(raw) : null,
    };
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: opts.roles ?? ["admin"],
          allow: opts.allow ?? PIPELINE_ADMIN,
        }),
      );
    }
    calls.push(call);
    if (url.endsWith("/pipelines/pl/stage-order")) {
      if (opts.stageOrderRefusal) {
        return jsonResponse(
          opts.stageOrderRefusal.body,
          opts.stageOrderRefusal.status,
        );
      }
      const ids: string[] = call.body.stage_ids;
      sales = ids.flatMap((id, index) => {
        const found = sales.find((s) => s.id === id);
        return found ? [{ ...found, position: index + 1 }] : [];
      });
      salesVersion += 1;
      return jsonResponse(salesPipeline());
    }
    if (url.endsWith("/pipelines/order")) {
      return jsonResponse({ data: catalog(), page: { has_more: false } });
    }
    if (url.endsWith("/pipelines/pl-old/restore") && method === "POST") {
      return jsonResponse({ ...catalog()[0], archived_at: null });
    }
    if (url.includes("/pipelines/") && method === "DELETE") {
      return new Response(null, { status: 204 });
    }
    if (url.endsWith("/pipelines/pl") && method === "GET") {
      return jsonResponse(salesPipeline());
    }
    if (
      url.includes("/pipelines") &&
      (method === "POST" || method === "PATCH")
    ) {
      return jsonResponse({
        id: "pl-new",
        name: "Partnerships",
        version: 1,
        ...call.body,
      });
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: catalog(), page: { next_cursor: null } });
    }
    if (url.endsWith("/stages") && method === "POST") {
      const created = stage(
        "s-new",
        call.body.name,
        call.body.position,
        call.body.semantic,
        call.body.win_probability,
      );
      sales = [...sales, created];
      salesVersion += 1;
      return jsonResponse(created, 201);
    }
    if (url.includes("/stages/") && method === "DELETE") {
      if (opts.stageDeleteRefusal) {
        return jsonResponse(
          opts.stageDeleteRefusal.body,
          opts.stageDeleteRefusal.status,
        );
      }
      return new Response(null, { status: 204 });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
  vi.stubGlobal("fetch", fetch);
  return {
    calls,
    writes: (method: string, path: string) =>
      calls.filter(
        (c) => c.method === method && new URL(c.url).pathname.endsWith(path),
      ),
  };
}

function renderPage() {
  return render(
    <ToastProvider>
      <PipelinesCard />
      <ToastRegion />
    </ToastProvider>,
  );
}

// The open ladder's order as its grips name it: one grip per row, in row order.
function openStageNames(): string[] {
  const list = screen.getByRole("list", { name: "Open stages" });
  return within(list)
    .getAllByRole("button", { name: /^Move / })
    .map(
      (grip) =>
        /^Move (.+), step/.exec(grip.getAttribute("aria-label") ?? "")?.[1] ??
        "",
    );
}

describe("PipelinesCard: who is shown what", () => {
  it("shows create controls for an admin", async () => {
    server({});
    renderPage();
    expect(await screen.findByText("New pipeline")).toBeTruthy();
  });

  it("gives a reader the catalog and the ladder, and no grip or verb", async () => {
    server({ roles: ["rep"], allow: {} });
    renderPage();
    await screen.findByRole("list", { name: "Open stages" });
    expect(screen.queryByText("New pipeline")).toBeNull();
    expect(screen.queryByRole("button", { name: /^Move / })).toBeNull();
    expect(screen.getByText(/Read-only/)).toBeTruthy();
  });

  // One grant at a time: create and update govern different controls, and a
  // fixture holding both cannot tell a correct binding from a transposed one.
  it("offers stage editing and ordering on update alone, without the create affordance", async () => {
    server({ allow: { pipeline: ["update"] } });
    renderPage();
    expect(await screen.findByTestId("new-stage-pl")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Move Proposal, step 2 of 2" }),
    ).toBeTruthy();
    expect(screen.queryByText("New pipeline")).toBeNull();
  });

  it("offers the create affordance on create alone, without stage editing or ordering", async () => {
    server({ allow: { pipeline: ["create"] } });
    renderPage();
    expect(await screen.findByText("New pipeline")).toBeTruthy();
    expect(screen.queryByTestId("new-stage-pl")).toBeNull();
    expect(screen.queryByRole("button", { name: /^Move / })).toBeNull();
  });

  // Removal and retiring are pipeline:delete, a different verb from everything
  // else here, so a principal holding update alone is shown neither.
  it("withholds stage removal and retiring from a principal holding update alone", async () => {
    server({ allow: { pipeline: ["read", "update"] } });
    renderPage();
    expect(await screen.findByTestId("new-stage-pl")).toBeTruthy();
    expect(screen.queryByTestId("remove-stage-s1")).toBeNull();
    expect(screen.queryByRole("button", { name: "Retire" })).toBeNull();
  });
});

describe("PipelinesCard: the catalog", () => {
  it("opens the default pipeline first, and another one on press, into the address", async () => {
    const user = userEvent.setup();
    server({});
    renderPage();
    expect(await screen.findByRole("heading", { name: "Sales" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /^Enterprise/ }));
    expect(
      await screen.findByRole("heading", { name: "Enterprise" }),
    ).toBeTruthy();
    expect(globalThis.location.hash).toContain("pipeline=pl-live");
  });

  // The deal board, the stage-automation picker and the lead qualifier read
  // ["pipelines","all"] with no include_archived. If this page widened that
  // entry instead of taking its own, a retirement would put the pipeline back
  // in the places it was retired FROM.
  it("asks for archived rows on a request of its own, and lists them apart", async () => {
    const backend = server({});
    renderPage();
    await screen.findByText(/1 retired pipeline/);
    expect(
      backend.calls.some((c) => c.url.includes("include_archived=true")),
    ).toBe(true);
    // Retired pipelines hold no place in the order.
    expect(
      screen.queryByRole("button", { name: /^Move Retired line/ }),
    ).toBeNull();
  });

  it("reorders the pipelines in use with the keyboard, naming only those", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    const grip = await screen.findByRole("button", {
      name: "Move Sales, 2 of 2",
    });
    grip.focus();
    await user.keyboard("{ArrowUp}");
    await waitFor(() =>
      expect(backend.writes("PUT", "/pipelines/order")).toHaveLength(1),
    );
    expect(backend.writes("PUT", "/pipelines/order")[0].body).toEqual({
      pipeline_ids: ["pl", "pl-live"],
    });
    expect(await screen.findByText("Order saved")).toBeTruthy();
  });

  it("creating a pipeline places it last with its closing pair, and opens it", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(await screen.findByText("New pipeline"));
    await user.type(screen.getByLabelText(/Name/), "Partnerships");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() =>
      expect(backend.writes("POST", "/pipelines")).toHaveLength(1),
    );
    expect(backend.writes("POST", "/pipelines")[0].body).toMatchObject({
      name: "Partnerships",
      position: 3,
      stages: [
        { semantic: "won", win_probability: 100 },
        { semantic: "lost", win_probability: 0 },
      ],
    });
    await waitFor(() =>
      expect(globalThis.location.hash).toContain("pipeline=pl-new"),
    );
  });
});

describe("PipelinesCard: the ladder", () => {
  // Each closing stage says what it MEANS beside its name: a ladder whose ends
  // are drawn like its middle leaves a reader to infer from the name whether
  // "Closed lost" counts as won.
  it("sets the closing pair apart and names each outcome", async () => {
    server({});
    renderPage();
    await screen.findByRole("list", { name: "Open stages" });
    expect(screen.getByRole("heading", { name: "Open stages" })).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Closing stages" }),
    ).toBeTruthy();
    expect(screen.getAllByText("Won").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Lost").length).toBeGreaterThan(0);
    // The closing pair is locked at the end: no grip carries it.
    expect(screen.queryByRole("button", { name: /^Move Closed/ })).toBeNull();
  });

  it("reorders open stages as one write naming the whole ladder, pinned to its version", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    const grip = await screen.findByRole("button", {
      name: "Move Proposal, step 2 of 2",
    });
    grip.focus();
    await user.keyboard("{ArrowUp}");
    // Drawn before the server answers: the row does not snap back first.
    expect(openStageNames()).toEqual(["Proposal", "Qualify"]);
    await waitFor(() =>
      expect(backend.writes("PUT", "/pipelines/pl/stage-order")).toHaveLength(
        1,
      ),
    );
    const [order] = backend.writes("PUT", "/pipelines/pl/stage-order");
    expect(order.ifMatch).toBe("3");
    expect(order.body).toEqual({ stage_ids: ["s1b", "s1", "s2", "s3"] });
    expect(screen.getByText("Proposal is now step 1 of 2")).toBeTruthy();
  });

  it("undoes an order from the pipeline as it stands after the save", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    (
      await screen.findByRole("button", { name: "Move Proposal, step 2 of 2" })
    ).focus();
    await user.keyboard("{ArrowUp}");
    await user.click(await screen.findByRole("button", { name: "Undo" }));
    await waitFor(() =>
      expect(backend.writes("PUT", "/pipelines/pl/stage-order")).toHaveLength(
        2,
      ),
    );
    const undo = backend.writes("PUT", "/pipelines/pl/stage-order")[1];
    expect(undo.body).toEqual({ stage_ids: ["s1", "s1b", "s2", "s3"] });
    // The save moved the version; the way back is pinned to the new one.
    expect(undo.ifMatch).toBe("4");
  });

  it("puts the old order back and says why when somebody changed the ladder first", async () => {
    const user = userEvent.setup();
    server({
      stageOrderRefusal: {
        status: 409,
        body: { status: 409, code: "version_skew", detail: "version skew" },
      },
    });
    renderPage();
    (
      await screen.findByRole("button", { name: "Move Proposal, step 2 of 2" })
    ).focus();
    await user.keyboard("{ArrowUp}");
    expect(
      await screen.findByText(/Someone else changed it first/),
    ).toBeTruthy();
    await waitFor(() =>
      expect(openStageNames()).toEqual(["Qualify", "Proposal"]),
    );
  });

  // The server puts a new open stage in front of the closing pair in the same
  // write, so adding one is one request and never a second one to place it.
  it("adds a stage at the end of the ladder in one write", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(await screen.findByTestId("new-stage-pl"));
    const form = screen.getByRole("dialog");
    await user.type(within(form).getByLabelText(/Name/), "Discovery");
    await user.type(within(form).getByLabelText(/Win probability/), "15");
    await user.click(within(form).getByRole("button", { name: "Create" }));
    await waitFor(() =>
      expect(backend.writes("POST", "/stages")).toHaveLength(1),
    );
    expect(backend.writes("POST", "/stages")[0].body).toMatchObject({
      pipeline_id: "pl",
      semantic: "open",
      win_probability: 15,
      position: 5,
    });
    expect(await screen.findByText("Stage added")).toBeTruthy();
    expect(backend.writes("PUT", "/pipelines/pl/stage-order")).toHaveLength(0);
  });

  it("points out a stage whose odds sit below the stage above it", async () => {
    const user = userEvent.setup();
    server({});
    renderPage();
    (
      await screen.findByRole("button", { name: "Move Proposal, step 2 of 2" })
    ).focus();
    await user.keyboard("{ArrowUp}");
    expect(
      await screen.findByText("Lower than Proposal (50%) above it"),
    ).toBeTruthy();
  });

  it("removes a stage through DELETE once the confirm is taken", async () => {
    const user = userEvent.setup();
    const backend = server({
      allow: { pipeline: ["read", "update", "delete"] },
    });
    renderPage();
    await user.click(await screen.findByTestId("remove-stage-s1"));
    expect(screen.getByText(/leaves the pipeline/).textContent).toContain(
      "Qualify",
    );
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    await waitFor(() =>
      expect(backend.writes("DELETE", "/stages/s1")).toHaveLength(1),
    );
  });

  // The refusal is the server's, and it names the deals in the way. The dialog
  // stays open showing it: a closed dialog would drop the only sentence
  // telling the admin what to move.
  it("shows the occupied-stage refusal and keeps the stage", async () => {
    const user = userEvent.setup();
    server({
      allow: { pipeline: ["read", "update", "delete"] },
      stageDeleteRefusal: {
        status: 422,
        body: {
          status: 422,
          code: "stage_occupied",
          detail:
            "1 deal(s) still sit on this stage: Acme rollout. Move them to another stage first.",
        },
      },
    });
    renderPage();
    await user.click(await screen.findByTestId("remove-stage-s1"));
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    expect(await screen.findByText(/Acme rollout/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove stage" })).toBeTruthy();
  });
});

describe("PipelinesCard: the pipeline's own verbs", () => {
  // The default's Retire stays VISIBLE and disabled with the reason beside it:
  // the reason names a remedy this same page offers.
  it("disables retire on the default pipeline and says why", async () => {
    server({ allow: { pipeline: ["read", "update", "delete"] } });
    renderPage();
    expect(
      await screen.findByText(/Make another pipeline the default first/),
    ).toBeTruthy();
  });

  it("retires a pipeline through DELETE with its version, once confirmed", async () => {
    const user = userEvent.setup();
    const backend = server({
      allow: { pipeline: ["read", "update", "delete"] },
    });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: /^Enterprise/ }),
    );
    await screen.findByRole("heading", { name: "Enterprise" });
    await user.click(await screen.findByRole("button", { name: "Retire" }));
    const confirm = screen.getByRole("dialog");
    expect(within(confirm).getByText(/keep their stage/).textContent).toContain(
      "Enterprise",
    );
    await user.click(within(confirm).getByRole("button", { name: "Retire" }));
    await waitFor(() =>
      expect(backend.writes("DELETE", "/pipelines/pl-live")).toHaveLength(1),
    );
    expect(backend.writes("DELETE", "/pipelines/pl-live")[0].ifMatch).toBe("7");
  });

  it("puts a retired pipeline back through restore, and draws its ladder read-only", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(await screen.findByText(/1 retired pipeline/));
    await user.click(screen.getByRole("button", { name: /^Retired line/ }));
    expect(
      await screen.findByText(/New deals cannot start in this pipeline/),
    ).toBeTruthy();
    expect(screen.queryByTestId("new-stage-pl-old")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Restore" }));
    await waitFor(() =>
      expect(backend.writes("POST", "/pipelines/pl-old/restore")).toHaveLength(
        1,
      ),
    );
  });

  it("makes a pipeline the default, pinned to its version", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: /^Enterprise/ }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Make default" }),
    );
    await waitFor(() =>
      expect(backend.writes("PATCH", "/pipelines/pl-live")).toHaveLength(1),
    );
    const [promote] = backend.writes("PATCH", "/pipelines/pl-live");
    expect(promote.body).toEqual({ is_default: true });
    expect(promote.ifMatch).toBe("7");
  });

  it("renaming a pipeline patches that pipeline", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(await screen.findByRole("button", { name: "Rename" }));
    const name = screen.getByLabelText(/Name/);
    await user.clear(name);
    await user.type(name, "Renamed");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(backend.writes("PATCH", "/pipelines/pl")).toHaveLength(1),
    );
    expect(backend.writes("PATCH", "/pipelines/pl")[0].body).toMatchObject({
      name: "Renamed",
    });
  });
});
