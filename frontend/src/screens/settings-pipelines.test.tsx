/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { PipelinesCard } from "./settings.pipelines";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The pipeline and stage editor the Data model entry carries. Each affordance
// follows its OWN verb — create, update, delete — so the cases below grant one
// at a time and read the card directly rather than through the entry that hosts
// it.

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// Routed by URL, with the pipelines list stubbed to the D-8 shape (an array
// with embedded stages) and a POST /stages hook so a test can inspect the exact
// body shipped.
function settingsStub(opts: {
  roles: string[];
  allow?: GrantSpec;
  onStagePost?: (body: unknown) => void;
  onStageDelete?: (url: string) => void;
  // What the server answers a removal with, when the scenario is about a
  // refusal: a stage still holding deals, or the terminal pair.
  stageDeleteRefusal?: { status: number; body: unknown };
  onPipelineWrite?: (call: { method: string; body: unknown }) => void;
  onPipelineArchive?: (call: { url: string; ifMatch: string | null }) => void;
  onPipelineRestore?: (call: { url: string; method: string }) => void;
  onPipelineList?: (url: string) => void;
}) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({ roles: opts.roles, allow: opts.allow ?? PIPELINE_ADMIN }),
      );
    }
    if (url.includes("/pipelines/") && url.endsWith("/restore")) {
      opts.onPipelineRestore?.({ url, method });
      return jsonResponse({
        id: "pl-old",
        name: "Retired line",
        is_default: false,
        position: 1,
        version: 4,
      });
    }
    if (url.includes("/pipelines/") && method === "DELETE") {
      opts.onPipelineArchive?.({
        url,
        ifMatch:
          input instanceof Request ? input.headers.get("If-Match") : null,
      });
      return new Response(null, { status: 204 });
    }
    if (
      url.includes("/pipelines") &&
      (method === "POST" || method === "PATCH")
    ) {
      const raw = input instanceof Request ? await input.clone().text() : "";
      const body = raw ? JSON.parse(raw) : {};
      opts.onPipelineWrite?.({ method, body });
      return jsonResponse({ id: "pl-new", name: "Enterprise", ...body });
    }
    if (url.includes("/pipelines")) {
      opts.onPipelineList?.(url);
      return jsonResponse({
        data: [
          {
            id: "pl-old",
            name: "Retired line",
            is_default: false,
            position: 1,
            version: 4,
            archived_at: "2026-09-01T00:00:00Z",
            stages: [],
          },
          // Live and NOT the default: the only shape that can actually be
          // retired, so a fixture without one would let the retire case pass
          // on a control the reader can never press.
          {
            id: "pl-live",
            name: "Enterprise",
            is_default: false,
            position: 2,
            version: 7,
            stages: [],
          },
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            version: 3,
            stages: [
              {
                id: "s1",
                pipeline_id: "pl",
                name: "Qualify",
                position: 1,
                semantic: "open",
                win_probability: 20,
              },
              // A ladder ends somewhere, and the two ends mean opposite
              // things. Without them the fixture only ever exercised the
              // middle, so what a reader is told a terminal stage IS went
              // unasserted.
              {
                id: "s2",
                pipeline_id: "pl",
                name: "Closed won",
                position: 2,
                semantic: "won",
                win_probability: 100,
              },
              {
                id: "s3",
                pipeline_id: "pl",
                name: "Closed lost",
                position: 3,
                semantic: "lost",
                win_probability: 0,
              },
            ],
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/stages") && method === "POST") {
      const raw = input instanceof Request ? await input.clone().text() : "";
      const body = raw ? JSON.parse(raw) : {};
      opts.onStagePost?.(body);
      return jsonResponse(body);
    }
    if (url.includes("/stages/") && method === "DELETE") {
      opts.onStageDelete?.(url);
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
}

describe("PipelinesCard", () => {
  it("shows create controls for an admin", async () => {
    vi.stubGlobal("fetch", settingsStub({ roles: ["admin"] }));
    render(<PipelinesCard />);
    expect(await screen.findByText("New pipeline")).toBeTruthy();
  });
  it("hides create controls for a rep", async () => {
    vi.stubGlobal("fetch", settingsStub({ roles: ["rep"], allow: {} }));
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    expect(screen.queryByText("New pipeline")).toBeNull();
  });
  // One grant at a time: create and update govern different controls, and a
  // fixture holding both cannot tell a correct binding from a transposed one.
  it("offers stage editing on update alone, without the create affordance", async () => {
    vi.stubGlobal(
      "fetch",
      settingsStub({ roles: ["admin"], allow: { pipeline: ["update"] } }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    expect(screen.getByTestId("new-stage-pl")).toBeTruthy();
    expect(screen.queryByText("New pipeline")).toBeNull();
  });

  it("offers the create affordance on create alone, without stage editing", async () => {
    vi.stubGlobal(
      "fetch",
      settingsStub({ roles: ["admin"], allow: { pipeline: ["create"] } }),
    );
    render(<PipelinesCard />);
    expect(await screen.findByText("New pipeline")).toBeTruthy();
    expect(screen.queryByTestId("new-stage-pl")).toBeNull();
  });

  // Removal is pipeline:delete, a different verb from everything else on
  // this card — a principal who may add and rename stages must not be
  // shown a control the server would only ever 403.
  it("withholds stage removal from a principal holding update alone", async () => {
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update"] },
      }),
    );
    render(<PipelinesCard />);
    expect(await screen.findByTestId("new-stage-pl")).toBeTruthy();
    expect(screen.queryByTestId("remove-stage-s1")).toBeNull();
  });

  it("removes a stage through DELETE once the confirm is taken", async () => {
    const user = userEvent.setup();
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update", "delete"] },
        onStageDelete: (url) => deleted.push(url),
      }),
    );
    render(<PipelinesCard />);
    await user.click(await screen.findByTestId("remove-stage-s1"));
    // The dialog names the stage, so the confirm is about a stage the
    // reader recognises rather than "this one".
    expect(screen.getByText(/leaves the pipeline/).textContent).toContain(
      "Qualify",
    );
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    await waitFor(() => expect(deleted).toHaveLength(1));
    expect(deleted[0]).toContain("/stages/s1");
  });

  // The refusal is the server's, and it names the deals in the way. The
  // dialog stays open showing it: a closed dialog would drop the only
  // sentence telling the admin what to move.
  it("shows the occupied-stage refusal and keeps the stage", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update", "delete"] },
        stageDeleteRefusal: {
          status: 422,
          // Exactly what the server sends: a MessageFault renders a
          // machine code and a reason, and no per-field details body —
          // a fixture that invented one would document a contract the
          // backend does not have.
          body: {
            type: "https://errors.gradion.com/stage_occupied",
            title: "Unprocessable Entity",
            status: 422,
            code: "stage_occupied",
            detail:
              "1 deal(s) still sit on this stage: Acme rollout. Move them to another stage first.",
          },
        },
      }),
    );
    render(<PipelinesCard />);
    await user.click(await screen.findByTestId("remove-stage-s1"));
    await user.click(screen.getByRole("button", { name: "Remove stage" }));
    expect(await screen.findByText(/Acme rollout/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove stage" })).toBeTruthy();
  });

  it("create stage posts the pipeline_id + semantic + win_probability", async () => {
    const user = userEvent.setup();
    const posts: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({ roles: ["admin"], onStagePost: (b) => posts.push(b) }),
    );
    render(<PipelinesCard />);
    await user.click(await screen.findByTestId("new-stage-pl"));
    await user.type(screen.getByLabelText(/Name/), "Discovery");
    await user.type(screen.getByLabelText(/Win probability/), "15");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() =>
      expect(posts[0]).toMatchObject({
        pipeline_id: "pl",
        semantic: "open",
        win_probability: 15,
      }),
    );
  });
  // The whole point of retiring a pipeline is that it stops being offered, so
  // the settings card is the ONE reader that must still see it. That the other
  // three readers do not is the claim below this one.
  it("lists a retired pipeline and marks it retired", async () => {
    vi.stubGlobal("fetch", settingsStub({ roles: ["admin"] }));
    render(<PipelinesCard />);
    expect(await screen.findByText("Retired line")).toBeTruthy();
    expect(screen.getByText("Retired")).toBeTruthy();
  });

  // The deal board, the stage-automation picker and the lead qualifier read
  // ["pipelines","all"] with no include_archived. If this card widened that
  // entry instead of taking its own, a retirement would put the pipeline back
  // in the three places it was retired FROM — silently, through a shared
  // cache, with nothing on this screen to show for it.
  it("asks for archived rows on a request of its own", async () => {
    const listed: string[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        onPipelineList: (url) => listed.push(url),
      }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Retired line");
    expect(listed.some((url) => url.includes("include_archived=true"))).toBe(
      true,
    );
  });

  // Retiring is pipeline:DELETE, the same verb stage removal takes and a
  // different one from everything else this row offers.
  it("withholds the retire verb from a principal holding update alone", async () => {
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update"] },
      }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    expect(screen.queryByRole("button", { name: "Retire" })).toBeNull();
  });

  // The default pipeline's control stays VISIBLE and disabled with the reason
  // beside it. The server refuses it (default_pipeline_not_archivable) and the
  // reason names a remedy the reader can take from this same row, so hiding
  // the control would hide the only sentence that explains the state.
  it("disables retire on the default pipeline and says why", async () => {
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update", "delete"] },
      }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    expect(
      screen.getByText(/Make another pipeline the default first/),
    ).toBeTruthy();
  });

  it("retires a pipeline through DELETE with its version, once confirmed", async () => {
    const user = userEvent.setup();
    const archived: { url: string; ifMatch: string | null }[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        allow: { pipeline: ["read", "update", "delete"] },
        onPipelineArchive: (call) => archived.push(call),
      }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    // Exactly one of the three can be retired, and saying so here is the
    // claim: the retired one offers no Retire verb at all, and the default's
    // is drawn and refused. Picking "the enabled one" is therefore a reading
    // of the rule rather than a way around an ambiguous query.
    const offered = screen
      .getAllByRole("button", { name: "Retire" })
      .filter((button) => !(button as HTMLButtonElement).disabled);
    expect(offered).toHaveLength(1);
    await user.click(offered[0]);
    expect(screen.getByText(/keep their stage/).textContent).toContain(
      "Enterprise",
    );
    // The dialog's own confirm, not the row's trigger: both carry the verb,
    // which is the point — the dialog repeats the word the reader pressed.
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Retire",
      }),
    );
    await waitFor(() => expect(archived).toHaveLength(1));
    expect(archived[0].url).toContain("/pipelines/pl-live");
    // The version travels: retiring a pipeline somebody else has just made
    // default is a decision taken about a record the reader was not looking at.
    expect(archived[0].ifMatch).toBe("7");
  });

  it("puts a retired pipeline back through restore", async () => {
    const user = userEvent.setup();
    const restored: { url: string; method: string }[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        onPipelineRestore: (call) => restored.push(call),
      }),
    );
    render(<PipelinesCard />);
    await user.click(
      await screen.findByRole("button", { name: "Put back in use" }),
    );
    await waitFor(() => expect(restored).toHaveLength(1));
    expect(restored[0].url).toContain("/pipelines/pl-old/restore");
    // The VERB as well as the address. Restoring is a POST — the contract's
    // own undo half — and a stub matching on the path alone would let a
    // regression to GET or DELETE pass while still hitting the right row.
    expect(restored[0].method).toBe("POST");
  });
  // Each stage says what it MEANS, not just what it is called. A ladder whose
  // ends are drawn like its middle leaves a reader to infer from the name
  // whether "Closed lost" counts as won — and a renamed stage takes that
  // inference with it.
  it("names each stage's outcome beside it", async () => {
    vi.stubGlobal("fetch", settingsStub({ roles: ["admin"] }));
    render(<PipelinesCard />);
    await screen.findByText("Qualify");

    expect(screen.getByText("Open")).toBeTruthy();
    expect(screen.getByText("Won")).toBeTruthy();
    expect(screen.getByText("Lost")).toBeTruthy();
  });
  // The card's own create verb, driven rather than merely asserted present.
  it("creating a pipeline posts the name and an empty ladder", async () => {
    const user = userEvent.setup();
    const writes: { method: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        onPipelineWrite: (w) => writes.push(w),
      }),
    );
    render(<PipelinesCard />);
    await user.click(await screen.findByText("New pipeline"));
    await user.type(screen.getByLabelText(/Name/), "Partnerships");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0].method).toBe("POST");
    // Stages are added afterwards, one at a time, through the row's own verb —
    // a create that shipped a ladder would be a second way to build one.
    expect(writes[0].body).toMatchObject({ name: "Partnerships", stages: [] });
  });

  // Renaming rides PATCH, and the row that carries it is the row it names.
  it("renaming a pipeline patches that pipeline", async () => {
    const user = userEvent.setup();
    const writes: { method: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      settingsStub({
        roles: ["admin"],
        onPipelineWrite: (w) => writes.push(w),
      }),
    );
    render(<PipelinesCard />);
    await screen.findByText("Sales");
    // An IconAction, so its accessible name is the label rather than text in
    // the row. The first is the retired pipeline's — the live default is next.
    await user.click(
      screen.getAllByRole("button", { name: "Edit pipeline" })[0],
    );
    const name = screen.getByLabelText(/Name/);
    await user.clear(name);
    await user.type(name, "Renamed");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0].method).toBe("PATCH");
    expect(writes[0].body).toMatchObject({ name: "Renamed" });
  });
});
