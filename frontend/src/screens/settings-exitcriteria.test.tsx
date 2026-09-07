/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { StageExitCriteria } from "./settings.exitcriteria";
import { jsonResponse, PIPELINE_ADMIN, render } from "./settings.testkit";

// The exit-criteria editor inside a stage's row. What a stage REQUIRES is
// configuration; whether a deal has met it is evidence, and none of that
// appears here.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

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
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit />);

    const first = await screen.findByText("Buyer confirmed the problem");
    const second = screen.getByText("Economic buyer identified");
    expect(
      first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();

    // Which one blocks is the fact a reader is here for.
    const rows = screen.getAllByRole("listitem");
    expect(within(rows[0]).getByText("Required")).toBeTruthy();
    expect(within(rows[1]).getByText("Optional")).toBeTruthy();
  });

  it("ships the exact body when a criterion is added", async () => {
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      criteriaStub({ onPost: (body) => posted.push(body) }),
    );
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit />);

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

  // Won and lost are where a deal stops. The server refuses a criterion there
  // too, but a control that exists only to be refused is worse than one that
  // is never drawn.
  it("refuses to offer criteria on a won or lost stage", async () => {
    for (const semantic of ["won", "lost"] as const) {
      vi.stubGlobal("fetch", criteriaStub({}));
      render(<StageExitCriteria stageId="s1" semantic={semantic} canEdit />);
      expect(await screen.findByText(/is where a deal stops/)).toBeTruthy();
      expect(screen.queryByTestId("new-criterion-s1")).toBeNull();
      cleanup();
    }
  });

  // The editor never offers the key. Evidence matches a criterion by key
  // across an edit, so renaming one would orphan every claim recorded under it.
  it("never offers the key on an edit", async () => {
    vi.stubGlobal("fetch", criteriaStub({}));
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit />);

    const rows = await screen.findAllByRole("listitem");
    await userEvent.click(
      within(rows[0]).getByRole("button", { name: /Edit criterion/ }),
    );
    expect(await screen.findByLabelText(/Label/)).toBeTruthy();
    expect(screen.queryByLabelText(/^Key/)).toBeNull();
  });

  // The write affordances follow pipeline:update. A reader who may only look
  // sees the criteria and none of the verbs.
  it("hides the editor from a reader without pipeline:update", async () => {
    vi.stubGlobal("fetch", criteriaStub({ allow: { pipeline: ["read"] } }));
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit={false} />);

    // The positive control: the list itself still renders, so the absences
    // below are about the verbs and not about a screen that never loaded.
    expect(await screen.findByText("Buyer confirmed the problem")).toBeTruthy();
    expect(screen.queryByTestId("new-criterion-s1")).toBeNull();
    expect(screen.queryByRole("button", { name: /Edit criterion/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Remove$/ })).toBeNull();
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
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit />);

    expect(await screen.findByText(/could not read/)).toBeTruthy();
    expect(screen.queryByText(/asks for nothing yet/)).toBeNull();
  });

  // Archiving keeps recorded evidence readable, so the confirmation says that
  // rather than promising a removal the server does not perform.
  it("says evidence survives when a criterion is removed", async () => {
    const deleted: string[] = [];
    vi.stubGlobal(
      "fetch",
      criteriaStub({ onDelete: (url) => deleted.push(url) }),
    );
    render(<StageExitCriteria stageId="s1" semantic="open" canEdit />);

    const rows = await screen.findAllByRole("listitem");
    await userEvent.click(
      within(rows[0]).getByRole("button", { name: /^Remove$/ }),
    );
    expect(await screen.findByText(/stays readable/)).toBeTruthy();

    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Remove$/,
      }),
    );
    await waitFor(() => expect(deleted).toHaveLength(1));
    expect(deleted[0]).toContain("/exit-criteria/c1");
  });
});
