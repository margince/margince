/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ProjectFilingAction } from "./projectfiling";
import { TimelineActions } from "./timelineactions";

type ProjectFiling = components["schemas"]["ProjectFiling"];
type Activity = components["schemas"]["Activity"];

const FILED: ProjectFiling = {
  filed: true,
  projects: [{ name: "ERP rollout", qualified_at: "2026-09-01T09:00:00Z" }],
  undoable: true,
  undone: [],
};

const UNFILED: ProjectFiling = {
  ...FILED,
  filed: false,
  projects: [],
  undoable: false,
  undone: [
    {
      id: "d1d1d1d1-0000-4000-8000-000000000001",
      at: "2026-09-02T10:30:00Z",
      by_name: "Ada Admin",
      reason: "the assistant filed the wrong thread",
      projects: ["ERP rollout"],
    },
  ],
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type":
        status >= 400 ? "application/problem+json" : "application/json",
    },
  });
}

type Sent = { key: string; body: unknown; headers: Headers };

function stubRoutes(routes: Record<string, () => Response>) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET" && request) {
        body = await request.clone().json();
      }
      sent.push({
        key,
        body,
        headers: request ? request.headers : new Headers(),
      });
      const route = routes[key];
      if (!route) throw new Error(`an unstubbed request: ${key}`);
      return route();
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
    queryClient: client,
  };
}

async function openDialog() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Undo filing" }));
  return { user, dialog: await screen.findByRole("dialog") };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ProjectFilingAction", () => {
  it("undoes the filing with a written reason and shows the decision it recorded", async () => {
    const sent = stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
      "POST /activities/act-1/project-filing/undo": () =>
        jsonResponse({
          ...UNFILED,
          undone: [{ ...UNFILED.undone[0], reason: "wrong thread" }],
        }),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { user, dialog } = await openDialog();

    // The server's verdict arrives before the form does.
    await within(dialog).findByText(/marked it as commercial correspondence/);
    const confirm = within(dialog).getByRole("button", { name: "Undo filing" });
    expect(
      confirm.getAttribute("aria-disabled") === "true" ||
        confirm.hasAttribute("disabled"),
    ).toBe(true);

    await user.type(within(dialog).getByRole("textbox"), "  wrong thread  ");
    await user.click(
      within(dialog).getByRole("button", { name: "Undo filing" }),
    );

    await within(dialog).findByText(/no longer filed under the project/);
    const undo = sent.find(
      (r) => r.key === "POST /activities/act-1/project-filing/undo",
    );
    expect(undo?.body).toEqual({ reason: "wrong thread" });
    // The audit entry is shown where the activity is: who, and in their words.
    expect(within(dialog).getByText(/Ada Admin/)).toBeTruthy();
    expect(within(dialog).getByText("wrong thread")).toBeTruthy();

    const closers = within(dialog).getAllByRole("button", { name: "Close" });
    await user.click(closers[closers.length - 1]);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("offers no undo, and says which rule stands in the way, when something else keeps the activity", async () => {
    const sent = stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        jsonResponse({
          ...FILED,
          undoable: false,
          refusal: { code: "other_basis_remains", message: "server words" },
        }),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    await within(dialog).findByText(
      /Something else still qualifies this activity/,
    );
    expect(within(dialog).queryByRole("textbox")).toBeNull();
    expect(sent.some((r) => r.key.startsWith("POST"))).toBe(false);
  });

  it("says there is nothing to undo for an activity no filing keeps, and lists earlier decisions", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(UNFILED),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    await within(dialog).findByText(/nothing to undo/);
    expect(
      within(dialog).getByText("the assistant filed the wrong thread"),
    ).toBeTruthy();
  });

  it("keeps the form and says why when the server refuses the undo", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
      "POST /activities/act-1/project-filing/undo": () =>
        jsonResponse(
          {
            title: "Conflict",
            code: "restricted",
            detail: "A hold has started.",
          },
          409,
        ),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { user, dialog } = await openDialog();

    await user.type(await within(dialog).findByRole("textbox"), "mistake");
    await user.click(
      within(dialog).getByRole("button", { name: "Undo filing" }),
    );

    await within(dialog).findByText(/A hold has started/);
    expect(within(dialog).getByRole("textbox")).toBeTruthy();
  });

  it("reports a filing that could not be read, as an alert with no form", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        jsonResponse({ title: "Forbidden", detail: "Not yours." }, 403),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("Not yours.");
    expect(within(dialog).queryByRole("textbox")).toBeNull();
  });

  it("does not show the last verdict while a fresh one is being read", async () => {
    let reads = 0;
    stubRoutes({
      "GET /activities/act-1/project-filing": () => {
        reads += 1;
        return reads === 1
          ? jsonResponse(FILED)
          : (new Promise<Response>(() => {}) as never);
      },
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { user, dialog } = await openDialog();
    await within(dialog).findByRole("textbox");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "Undo filing" }));
    const reopened = await screen.findByRole("dialog");
    expect((await within(reopened).findByRole("status")).textContent).toMatch(
      /Checking what keeps/,
    );
    expect(within(reopened).queryByRole("textbox")).toBeNull();
  });

  it("moves focus to the reason field when the verdict arrives", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();
    const box = await within(dialog).findByRole("textbox");
    await waitFor(() => expect(document.activeElement).toBe(box));
  });

  it("announces the wait while the verdict is read", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        new Promise<Response>(() => {}) as never,
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    expect((await within(dialog).findByRole("status")).textContent).toMatch(
      /Checking what keeps/,
    );
  });

  it("keeps the confirm refused for a reason of whitespace, by the one mechanism the button has", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { user, dialog } = await openDialog();

    await user.type(await within(dialog).findByRole("textbox"), "   ");
    const confirm = within(dialog).getByRole("button", { name: "Undo filing" });
    // One mechanism: the native attribute, with the sentence it is described by.
    expect(confirm.hasAttribute("disabled")).toBe(true);
    const sentence = within(dialog).getByText(/Write why the filing is wrong/);
    expect(confirm.getAttribute("aria-describedby")).toBe(sentence.id);

    await user.type(within(dialog).getByRole("textbox"), "x");
    expect(
      within(dialog)
        .getByRole("button", { name: "Undo filing" })
        .hasAttribute("disabled"),
    ).toBe(false);
  });

  it("sends one undo however often the confirm is pressed, and cannot be dismissed mid-flight", async () => {
    let release: (response: Response) => void = () => {};
    const sent = stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
      "POST /activities/act-1/project-filing/undo": () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }) as never,
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { user, dialog } = await openDialog();
    await user.type(await within(dialog).findByRole("textbox"), "mistake");

    const confirm = within(dialog).getByRole("button", { name: "Undo filing" });
    await user.click(confirm);
    await user.click(confirm);
    await waitFor(() =>
      expect(sent.filter((r) => r.key.startsWith("POST"))).toHaveLength(1),
    );

    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeNull();

    release(jsonResponse(UNFILED));
    await within(dialog).findByText(/no longer filed under the project/);
    expect(sent.filter((r) => r.key.startsWith("POST"))).toHaveLength(1);
  });

  it("refreshes the timeline when the dialog closes, not when the undo lands", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () => jsonResponse(FILED),
      "POST /activities/act-1/project-filing/undo": () => jsonResponse(UNFILED),
    });
    const { queryClient } = render(
      <ProjectFilingAction activityId="act-1" projectId="p-1" />,
    );
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { user, dialog } = await openDialog();
    await user.type(await within(dialog).findByRole("textbox"), "mistake");
    await user.click(
      within(dialog).getByRole("button", { name: "Undo filing" }),
    );
    await within(dialog).findByText(/no longer filed under the project/);
    expect(invalidate).not.toHaveBeenCalled();

    const closers = within(dialog).getAllByRole("button", { name: "Close" });
    await user.click(closers[closers.length - 1]);
    await waitFor(() => expect(invalidate).toHaveBeenCalled());
    // The timeline refetches by prefix; the activity's own read is exact.
    const exact = invalidate.mock.calls.map(
      ([filters]) => filters?.exact === true,
    );
    expect(exact).toContain(false);
    expect(exact).toContain(true);
  });

  it("names a hidden project as unnamed and shows a redacted decision as its moment alone", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        jsonResponse({
          ...FILED,
          projects: [
            { name: "", hidden: true, qualified_at: "2026-09-01T09:00:00Z" },
          ],
          undoable: false,
          refusal: { code: "hidden_project", message: "" },
          undone: [
            {
              id: "d2d2d2d2-0000-4000-8000-000000000002",
              at: "2026-09-02T10:30:00Z",
              redacted: true,
              by_name: "",
              reason: "",
              projects: [],
            },
          ],
        }),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    await within(dialog).findByText(
      /A project you cannot see still holds this activity/,
    );
    expect(within(dialog).getByText(/A decision was recorded on/)).toBeTruthy();
    expect(within(dialog).queryByText(/Ada Admin/)).toBeNull();
  });

  it("names the legal hold when one stands in the way", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        jsonResponse({
          ...FILED,
          undoable: false,
          refusal: { code: "legal_hold", message: "" },
        }),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    await within(dialog).findByText(/A legal hold sits on a record/);
    expect(within(dialog).queryByRole("textbox")).toBeNull();
  });
});

describe("TimelineActions", () => {
  const activity: Activity = {
    id: "act-1",
    kind: "note",
    subject: "Milestone",
    occurred_at: "2026-07-01T00:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };

  it("offers the undo on a project's timeline and nowhere else", () => {
    stubRoutes({});
    render(
      <TimelineActions
        activity={activity}
        entityType="project"
        entityId="p-1"
      />,
    );
    // Named by the row it acts on, so a column of them is not a column of one label.
    expect(
      screen.getByRole("button", { name: "Undo filing: Milestone" }),
    ).toBeTruthy();
    cleanup();
    render(
      <TimelineActions activity={activity} entityType="deal" entityId="d-1" />,
    );
    expect(screen.queryByRole("button", { name: /Undo filing/ })).toBeNull();
  });
});
