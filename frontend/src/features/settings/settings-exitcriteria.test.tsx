/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { StageExitCriteria } from "./settings.exitcriteria";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The exit-criteria editor inside a stage's row. What a stage REQUIRES is
// configuration; whether a deal has met it is evidence, and none of that
// appears here.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

function criterionRows(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>(".settingrow"));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

const CRITERIA = [
  {
    id: "c2",
    stage_id: "s1",
    key: "economic_buyer",
    label: "Economic buyer identified",
    kind: "role_identified",
    required: false,
    hint: null,
    position: 1,
    version: 1,
  },
  {
    id: "c1",
    stage_id: "s1",
    key: "buyer_confirmed",
    label: "Buyer confirmed the problem",
    kind: "buyer_confirmed",
    required: true,
    hint: null,
    position: 0,
    version: 1,
  },
];

function criteriaStub(opts: {
  allow?: GrantSpec;
  criteria?: unknown[];
  onPost?: (body: unknown) => void;
  onPatch?: (body: unknown) => void;
  onDelete?: (url: string) => void;
  onRead?: () => void;
}) {
  return vi.fn(async (input: RequestInfo | URL) => {
    // openapi-fetch dispatches a Request, so the method never rides an init
    // argument — reading one would report GET for every call.
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: ["admin"],
          allow: opts.allow ?? PIPELINE_ADMIN,
        }),
      );
    }
    if (url.includes("/exit-criteria") && method === "POST") {
      const raw = input instanceof Request ? await input.clone().text() : "";
      opts.onPost?.(raw ? JSON.parse(raw) : {});
      return jsonResponse({}, 201);
    }
    if (url.includes("/exit-criteria/") && method === "PATCH") {
      const raw = input instanceof Request ? await input.clone().text() : "";
      opts.onPatch?.(raw ? JSON.parse(raw) : {});
      return jsonResponse({});
    }
    if (url.includes("/exit-criteria/") && method === "DELETE") {
      opts.onDelete?.(url);
      return new Response(null, { status: 204 });
    }
    if (url.includes("/exit-criteria")) {
      opts.onRead?.();
      return jsonResponse({
        data: opts.criteria ?? CRITERIA,
        page: { next_cursor: null },
      });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

describe("StageExitCriteria", () => {
  // Position order, not the order the server happened to return: the fixture
  // is deliberately out of order so a component rendering it as received
  // would fail this.
  it("lists a stage's criteria in position order with required marked", async () => {
    vi.stubGlobal("fetch", criteriaStub({}));
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    const first = await screen.findByText("Buyer confirmed the problem");
    const second = screen.getByText("Economic buyer identified");
    expect(
      first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    // Which one blocks is the fact a reader is here for.
    const rows = criterionRows();
    expect(within(rows[0]).getByText("Required")).toBeTruthy();
    expect(within(rows[1]).getByText("Optional")).toBeTruthy();
  });

  // The key is an operator's word that evidence cites. Only the create form
  // asks for it.
  it("names each criterion by label and kind, never by its key", async () => {
    vi.stubGlobal("fetch", criteriaStub({}));
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    const label = await screen.findByText("Economic buyer identified");
    const row = label.closest<HTMLElement>(".settingrow");
    if (!row) throw new Error("the criterion drew no row");
    expect(within(row).getByText("Role identified")).toBeTruthy();
    expect(row.textContent).not.toContain("economic_buyer");
  });

  it("says a stage asks for nothing yet beside the verb that adds the first", async () => {
    vi.stubGlobal("fetch", criteriaStub({ criteria: [] }));
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    const none = await screen.findByText("No exit criteria yet.");
    expect(none.parentElement).toBe(
      screen.getByTestId("new-criterion-s1").parentElement,
    );
  });

  it("ships the exact body when a criterion is added", async () => {
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      criteriaStub({ onPost: (body) => posted.push(body) }),
    );
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    await userEvent.click(await screen.findByTestId("new-criterion-s1"));
    await userEvent.type(await screen.findByLabelText(/Key/), "demo_held");
    await userEvent.type(screen.getByLabelText(/Label/), "Demo held");
    await userEvent.click(screen.getByRole("combobox", { name: /Kind/ }));
    await userEvent.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Event held",
      }),
    );
    await userEvent.click(screen.getByRole("button", { name: /^Create$/ }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      key: "demo_held",
      label: "Demo held",
      kind: "event_held",
      required: true,
    });
  });

  // Evidence matches a criterion by key across an edit, so the editor names the
  // key under the label and offers no field to change it.
  it("shows the key on an edit and never offers to change it", async () => {
    vi.stubGlobal("fetch", criteriaStub({}));
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    await screen.findByText("Buyer confirmed the problem");
    const rows = criterionRows();
    await userEvent.click(
      within(rows[0]).getByRole("button", {
        name: "Edit Buyer confirmed the problem",
      }),
    );
    const label = await screen.findByLabelText(/Label/);
    const hints = (label.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => document.getElementById(id));
    const keyHint = hints.find((hint) => hint?.querySelector("code"));
    expect(keyHint?.textContent).toBe("Key: buyer_confirmed");
    expect(keyHint?.querySelector("code")?.textContent).toBe("buyer_confirmed");
    expect(screen.queryByLabelText(/^Key/)).toBeNull();
  });

  // The write affordances follow pipeline:update. A reader who may only look
  // sees the criteria and none of the verbs.
  it("hides the editor from a reader without pipeline:update", async () => {
    vi.stubGlobal("fetch", criteriaStub({ allow: { pipeline: ["read"] } }));
    render(
      <StageExitCriteria stageId="s1" stageName="Qualify" canEdit={false} />,
    );

    // The positive control: the list itself still renders, so the absences
    // below are about the verbs and not about a screen that never loaded.
    expect(await screen.findByText("Buyer confirmed the problem")).toBeTruthy();
    expect(screen.queryByTestId("new-criterion-s1")).toBeNull();
    expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Remove / })).toBeNull();
  });

  // A failed read is not an empty stage. Reporting one as the other would have
  // an admin re-adding criteria that are already there.
  it("says the criteria could not be read rather than that there are none", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({ roles: ["admin"], allow: PIPELINE_ADMIN }),
          );
        }
        if (url.includes("/exit-criteria")) {
          return jsonResponse({ title: "boom" }, 500);
        }
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }),
    );
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    expect(await screen.findByText(/Criteria did not load/)).toBeTruthy();
    expect(screen.queryByText(/No exit criteria yet/)).toBeNull();
  });

  // Archiving keeps recorded evidence readable, so the confirmation says that
  // rather than promising a removal the server does not perform.
  it("says evidence survives when a criterion is removed, and re-reads the list", async () => {
    const deleted: string[] = [];
    let reads = 0;
    vi.stubGlobal(
      "fetch",
      criteriaStub({
        onDelete: (url) => deleted.push(url),
        onRead: () => {
          reads += 1;
        },
      }),
    );
    render(<StageExitCriteria stageId="s1" stageName="Qualify" canEdit />);

    await screen.findByText("Buyer confirmed the problem");
    const rows = criterionRows();
    await userEvent.click(
      within(rows[0]).getByRole("button", {
        name: "Remove Buyer confirmed the problem",
      }),
    );
    expect(await screen.findByText(/stays readable/)).toBeTruthy();

    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Remove$/,
      }),
    );
    await waitFor(() => expect(deleted).toHaveLength(1));
    expect(deleted[0]).toContain("/exit-criteria/c1");
    await waitFor(() => expect(reads).toBe(2));
  });
});
