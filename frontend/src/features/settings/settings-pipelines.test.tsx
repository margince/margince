/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  jsonResponse,
  PIPELINE_ADMIN,
  render,
} from "../../testing/settings.testkit";
import { PipelinesCard } from "./settings.pipelines";

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
  extra: Readonly<Record<string, unknown>> = {},
) {
  return {
    id,
    pipeline_id: "pl",
    name,
    position,
    semantic,
    win_probability,
    version: position + 4,
    ...extra,
  };
}

// The server, as far as this page talks to it: the catalog and the Sales
// ladder are STATE, so a saved order is what the next read answers — which is
// what an Undo drawn from the latest version has to be tested against.
function server(opts: {
  roles?: string[];
  allow?: GrantSpec;
  stageOrderRefusal?: { status: number; body: unknown };
  stageDeleteRefusal?: { status: number; body: unknown };
  // The first DELETE finds the stage moved on: refused, and its version bumped.
  stageDeleteSkew?: boolean;
  // The re-read after that refusal waits for `release()`.
  holdReread?: boolean;
  onlyStage?: boolean;
  dealCount?: number;
  soloInUse?: boolean;
  // Sales and nothing else: no retired pipeline beside it either.
  onlyPipeline?: boolean;
  countsWithheld?: boolean;
}) {
  const calls: Call[] = [];
  let salesVersion = 3;
  let skewed = false;
  let reread = Promise.resolve();
  let release = () => {};
  let sales = [
    stage("s1", "Qualify", 1, "open", 20, {
      ...(opts.countsWithheld
        ? {}
        : { exit_criteria: { required: 2, optional: 1 } }),
      ...(opts.dealCount === undefined ? {} : { deal_count: opts.dealCount }),
    }),
    stage("s1b", "Proposal", 2, "open", 50, {
      exit_criteria: { required: 0, optional: 0 },
    }),
    stage("s2", "Closed won", 3, "won", 100),
    stage("s3", "Lost", 4, "lost", 0),
  ];
  if (opts.onlyStage) sales = sales.slice(0, 1);
  const salesPipeline = () => ({
    id: "pl",
    name: "Sales",
    is_default: true,
    position: 0,
    version: salesVersion,
    stages: sales,
  });
  const retiredLine = {
    id: "pl-old",
    name: "Retired line",
    is_default: false,
    position: 1,
    version: 4,
    archived_at: "2026-09-01T00:00:00Z",
    stages: [],
  };
  const catalog = () => [
    ...(opts.onlyPipeline ? [] : [retiredLine]),
    // Live and NOT the default: the only shape that can actually be retired.
    ...(opts.soloInUse
      ? []
      : [
          {
            id: "pl-live",
            name: "Enterprise",
            is_default: false,
            position: 2,
            version: 7,
            stages: [],
          },
        ]),
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
      await reread;
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
    if (url.includes("/exit-criteria")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/stages/") && method === "PATCH") {
      const id = new URL(url).pathname.split("/").pop();
      const found = sales.find((s) => s.id === id);
      return jsonResponse({ ...found, ...call.body });
    }
    if (url.includes("/stages/") && method === "DELETE") {
      const id = new URL(url).pathname.split("/").pop();
      if (opts.stageDeleteRefusal) {
        return jsonResponse(
          opts.stageDeleteRefusal.body,
          opts.stageDeleteRefusal.status,
        );
      }
      if (opts.stageDeleteSkew && !skewed) {
        skewed = true;
        sales = sales.map((s) =>
          s.id === id ? { ...s, version: s.version + 4 } : s,
        );
        if (opts.holdReread) {
          reread = new Promise((resolve) => {
            release = resolve;
          });
        }
        return jsonResponse(
          { status: 409, code: "version_skew", detail: "version skew" },
          409,
        );
      }
      sales = sales.filter((s) => s.id !== id);
      salesVersion += 1;
      return new Response(null, { status: 204 });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
  vi.stubGlobal("fetch", fetch);
  return {
    calls,
    release: () => release(),
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
  const table = screen.getByRole("table", { name: "Stages" });
  return within(table)
    .getAllByRole("button", { name: /^Move / })
    .map(
      (grip) =>
        /^Move (.+), step/.exec(grip.getAttribute("aria-label") ?? "")?.[1] ??
        "",
    );
}

// A refused button is disabled and points at its reason, which a reader sees.
function expectRefused(button: HTMLElement, reason: string) {
  expect((button as HTMLButtonElement).disabled).toBe(true);
  const describedBy = button.getAttribute("aria-describedby") ?? "";
  expect(document.getElementById(describedBy)?.textContent).toBe(reason);
}

describe("PipelinesCard: who is shown what", () => {
  it("shows create controls for an admin", async () => {
    server({});
    renderPage();
    expect(await screen.findByText("New pipeline")).toBeTruthy();
  });

  it("gives a reader the catalog and the ladder, and no grip or row verb", async () => {
    server({ roles: ["rep"], allow: {} });
    renderPage();
    await screen.findByRole("table", { name: "Stages" });
    expect(screen.queryByRole("button", { name: /^Move / })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Actions for / })).toBeNull();
    // The settings page says read-only once, above every card; the card adds none.
    expect(screen.queryByText(/Read-only/)).toBeNull();
  });

  // A hidden create verb reads as "this is all there can be". A reader who
  // sees one pipeline and no button concludes the product holds one.
  it("keeps the create verbs in sight for a reader, refused with the reason", async () => {
    server({ roles: ["rep"], allow: {} });
    renderPage();
    expectRefused(
      await screen.findByRole("button", { name: /New pipeline/ }),
      "Your role cannot create pipelines. Ask an admin.",
    );
    expectRefused(
      screen.getByTestId("new-stage-pl"),
      "Your role cannot add stages. Ask an admin.",
    );
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
    expectRefused(
      screen.getByRole("button", { name: /New pipeline/ }),
      "Your role cannot create pipelines. Ask an admin.",
    );
    expect(
      (screen.getByTestId("new-stage-pl") as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("offers the create affordance on create alone, without stage editing or ordering", async () => {
    server({ allow: { pipeline: ["create"] } });
    renderPage();
    const create = await screen.findByRole("button", { name: /New pipeline/ });
    expect((create as HTMLButtonElement).disabled).toBe(false);
    expectRefused(
      screen.getByTestId("new-stage-pl"),
      "Your role cannot add stages. Ask an admin.",
    );
    expect(screen.queryByRole("button", { name: /^Move / })).toBeNull();
  });

  // Removal and retiring are pipeline:delete, a different verb from everything
  // else here, so a principal holding update alone is shown neither.
  it("withholds stage removal and retiring from a principal holding update alone", async () => {
    const user = userEvent.setup();
    server({ allow: { pipeline: ["read", "update"] } });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    expect(screen.getByRole("button", { name: "Edit stage" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
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

  it("stands a lone pipeline without a catalog, its own head offering the next", async () => {
    server({ soloInUse: true, onlyPipeline: true });
    renderPage();
    const sales = await screen.findByRole("region", { name: "Sales" });
    expect(within(sales).getByText("New pipeline")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "All pipelines" })).toBeNull();
  });

  it("tells a reader of a lone pipeline that more are possible, just not by them", async () => {
    server({ roles: ["rep"], allow: {}, soloInUse: true, onlyPipeline: true });
    renderPage();
    const sales = await screen.findByRole("region", { name: "Sales" });
    expectRefused(
      within(sales).getByRole("button", { name: /New pipeline/ }),
      "Your role cannot create pipelines. Ask an admin.",
    );
  });

  // One pipeline in use has nothing to be ordered against: no grip, and no
  // hint for a grip that is not there.
  it("draws no grip and no order hint while one pipeline is in use", async () => {
    server({ soloInUse: true });
    renderPage();
    await screen.findByRole("table", { name: "Stages" });
    expect(screen.queryByRole("button", { name: /^Move Sales/ })).toBeNull();
    expect(screen.queryByText(/Drag a pipeline/)).toBeNull();
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
  // One table, the closing pair last. Each closing stage says what it means
  // beside its name only where the name does not already say it.
  it("draws the closing pair locked at the end, naming an outcome the name hides", async () => {
    server({});
    renderPage();
    const table = await screen.findByRole("table", { name: "Stages" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((row) => row.dataset.testid)).toEqual([
      "stage-s1",
      "stage-s1b",
      "stage-s2",
      "stage-s3",
    ]);
    const won = screen.getByTestId("stage-s2");
    expect(within(won).getByRole("img", { name: "Stays last" })).toBeTruthy();
    expect(within(won).getByText("Won")).toBeTruthy();
    expect(within(won).getByText("Ends the deal")).toBeTruthy();
    // "Lost" already says it: no second "Lost" beside it.
    expect(
      within(screen.getByTestId("stage-s3")).getAllByText("Lost"),
    ).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /^Move Closed/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Move Lost/ })).toBeNull();
  });

  it("counts each open stage's exit criteria, and says none where there are none", async () => {
    server({});
    renderPage();
    const qualify = await screen.findByTestId("stage-s1");
    expect(within(qualify).getByText("2 required · 1 optional")).toBeTruthy();
    expect(
      within(screen.getByTestId("stage-s1b")).getByText("No criteria"),
    ).toBeTruthy();
  });

  it("leaves a count the server did not send as unknown, never as none", async () => {
    server({ countsWithheld: true });
    renderPage();
    const qualify = await screen.findByTestId("stage-s1");
    expect(within(qualify).getByText("—")).toBeTruthy();
    expect(within(qualify).queryByText("No criteria")).toBeNull();
  });

  // The editor reads its stage's criteria only once a reader opens it: a
  // ladder of ten stages is not ten reads on arrival.
  it("opens a stage's exit criteria in place, reading them only then", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    const toggle = await screen.findByRole("button", {
      name: "Exit criteria for Qualify",
    });
    const criteriaReads = () =>
      backend.calls.filter((c) => c.url.includes("/exit-criteria"));
    expect(criteriaReads()).toHaveLength(0);
    await user.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    const add = await screen.findByRole("button", { name: "Add criterion" });
    expect(
      document.getElementById(add.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe("Qualify");
    expect(criteriaReads().map((c) => new URL(c.url).pathname)).toEqual([
      "/v1/stages/s1/exit-criteria",
    ]);
    expect(
      screen.queryByRole("button", { name: "Exit criteria for Closed won" }),
    ).toBeNull();
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
    expect(
      screen.getByRole("button", { name: "Move Proposal, step 1 of 2" }),
    ).toBe(document.activeElement);
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

  it("edits a stage from its row menu, pinned to the version it opened on", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Edit stage" }));
    const form = await screen.findByRole("dialog");
    const name = within(form).getByLabelText(/Name/);
    await user.clear(name);
    await user.type(name, "Qualified");
    await user.click(within(form).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(backend.writes("PATCH", "/stages/s1")).toHaveLength(1),
    );
    const [edit] = backend.writes("PATCH", "/stages/s1");
    expect(edit.ifMatch).toBe("5");
    expect(edit.body).toMatchObject({ name: "Qualified" });
  });

  it("removes a stage through DELETE pinned to its version, and hands focus to the next row", async () => {
    const user = userEvent.setup();
    const backend = server({
      allow: { pipeline: ["read", "update", "delete"] },
    });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const confirm = screen.getByRole("dialog");
    expect(
      within(confirm).getByText(/leaves the pipeline/).textContent,
    ).toContain("Qualify");
    await user.click(
      within(confirm).getByRole("button", { name: "Remove stage" }),
    );
    await waitFor(() =>
      expect(backend.writes("DELETE", "/stages/s1")).toHaveLength(1),
    );
    expect(backend.writes("DELETE", "/stages/s1")[0].ifMatch).toBe("5");
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Actions for Proposal" }),
      ),
    );
  });

  // A removal is refused on every live deal. A stage holding deals the reader
  // can see is refused in the menu, before any dialog opens.
  it("refuses removal in the menu while the stage holds deals, and says how many", async () => {
    const user = userEvent.setup();
    server({ allow: { pipeline: ["read", "update", "delete"] }, dealCount: 2 });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    const remove = screen.getByRole("button", { name: "Remove" });
    expect(remove.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText("2 deals are in this stage")).toBeTruthy();
  });

  // Without a deal grant the count is absent, not zero. Remove stays offered,
  // and the open dialog shows the server's refusal naming the deals.
  it("shows the occupied-stage refusal and keeps the stage when the count is unknown", async () => {
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
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    expect(await screen.findByText(/Acme rollout/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove stage" })).toBeTruthy();
  });

  it("adds a criterion from the row menu, with the stage's criteria open behind it", async () => {
    const user = userEvent.setup();
    server({});
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Add criterion" }));
    expect(
      await screen.findByRole("dialog", { name: "Add criterion" }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Exit criteria for Qualify" })
        .getAttribute("aria-expanded"),
    ).toBe("true");
  });

  // A stale version re-reads the ladder, so pressing the confirm again pins
  // the version the server now holds.
  it("re-reads the ladder when a removal finds the stage changed, and retries on the new version", async () => {
    const user = userEvent.setup();
    const backend = server({
      allow: { pipeline: ["read", "update", "delete"] },
      stageDeleteSkew: true,
    });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    expect(await screen.findByText(/changed after you opened it/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    await waitFor(() =>
      expect(backend.writes("DELETE", "/stages/s1")).toHaveLength(2),
    );
    const [refused, retried] = backend.writes("DELETE", "/stages/s1");
    expect(refused.ifMatch).toBe("5");
    expect(retried.ifMatch).toBe("9");
    await waitFor(() => expect(screen.queryByText("Qualify")).toBeNull());
  });

  it("holds the confirm while the re-read is out, so a second press cannot resend the stale version", async () => {
    const user = userEvent.setup();
    const backend = server({
      allow: { pipeline: ["read", "update", "delete"] },
      stageDeleteSkew: true,
      holdReread: true,
    });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const confirm = screen.getByRole("button", { name: "Remove stage" });
    await user.click(confirm);
    await waitFor(() =>
      expect(backend.writes("DELETE", "/stages/s1")).toHaveLength(1),
    );
    await waitFor(() => expect(confirm.getAttribute("aria-busy")).toBe("true"));
    await user.click(confirm);
    backend.release();
    expect(await screen.findByText(/changed after you opened it/)).toBeTruthy();
    expect(confirm.getAttribute("aria-busy")).toBeNull();
    expect(backend.writes("DELETE", "/stages/s1")).toHaveLength(1);
  });

  // With no row left holding a menu, focus lands on the group, never on <body>.
  it("hands focus to the heading when a delete-only seat removes the last open stage", async () => {
    const user = userEvent.setup();
    server({ allow: { pipeline: ["read", "delete"] } });
    renderPage();
    for (const name of ["Qualify", "Proposal"]) {
      await user.click(
        await screen.findByRole("button", { name: `Actions for ${name}` }),
      );
      await user.click(screen.getByRole("button", { name: "Remove" }));
      await user.click(screen.getByRole("button", { name: "Remove stage" }));
      await waitFor(() => expect(screen.queryByText(name)).toBeNull());
    }
    expect(screen.queryByRole("button", { name: /^Actions for / })).toBeNull();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("heading", { name: "Stages" }).firstElementChild,
      ),
    );
  });

  it("hands focus to New stage when the only stage is removed", async () => {
    const user = userEvent.setup();
    server({
      allow: { pipeline: ["read", "update", "delete"] },
      onlyStage: true,
    });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Qualify" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    await waitFor(() =>
      expect(screen.queryByRole("table", { name: "Stages" })).toBeNull(),
    );
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId("new-stage-pl")),
    );
  });

  // The criteria are read apart from the pipelines, so a reorder leaves an
  // open editor alone.
  it("does not re-read an open stage's criteria when the ladder is reordered", async () => {
    const user = userEvent.setup();
    const backend = server({});
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Exit criteria for Qualify" }),
    );
    const criteriaReads = () =>
      backend.calls.filter((c) => c.url.includes("/stages/s1/exit-criteria"))
        .length;
    await waitFor(() => expect(criteriaReads()).toBe(1));
    const reads = () =>
      backend.calls.filter(
        (c) => c.method === "GET" && c.url.includes("/pipelines"),
      ).length;
    const before = reads();
    screen.getByRole("button", { name: "Move Proposal, step 2 of 2" }).focus();
    await user.keyboard("{ArrowUp}");
    await waitFor(() =>
      expect(backend.writes("PUT", "/pipelines/pl/stage-order")).toHaveLength(
        1,
      ),
    );
    await waitFor(() => expect(reads()).toBeGreaterThan(before));
    expect(criteriaReads()).toBe(1);
  });

  it("offers no criteria and no removal on a closing stage", async () => {
    const user = userEvent.setup();
    server({ allow: { pipeline: ["read", "update", "delete"] } });
    renderPage();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Closed won" }),
    );
    expect(screen.getByRole("button", { name: "Edit stage" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add criterion" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
  });
});

describe("PipelinesCard: the pipeline's own verbs", () => {
  // The default's Retire stays VISIBLE and disabled, with the reason in the
  // row's description: it names a remedy this same page offers, and set beside
  // the button it took the whole row and crushed the description to a letter.
  it("disables retire on the default pipeline and says why under the row", async () => {
    server({ allow: { pipeline: ["read", "update", "delete"] } });
    renderPage();
    const reason = await screen.findByText(/Make another one the default/);
    expect(reason.closest(".settingrow-description")).not.toBeNull();
    const retire = screen.getByRole("button", { name: "Retire" });
    expect(retire.hasAttribute("disabled")).toBe(true);
    expect(retire.getAttribute("aria-describedby")).toBe(reason.id);
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
