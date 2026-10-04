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
      return (routes[key] ?? (() => jsonResponse({})))();
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
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
    expect(undo?.headers.get("Idempotency-Key")).toBeTruthy();
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

  it("reports a filing that could not be read", async () => {
    stubRoutes({
      "GET /activities/act-1/project-filing": () =>
        jsonResponse({ title: "Forbidden", detail: "Not yours." }, 403),
    });
    render(<ProjectFilingAction activityId="act-1" projectId="p-1" />);
    const { dialog } = await openDialog();

    await waitFor(() =>
      expect(within(dialog).queryByText(/Checking what keeps/)).toBeNull(),
    );
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
    expect(screen.getByRole("button", { name: "Undo filing" })).toBeTruthy();
    cleanup();
    render(
      <TimelineActions activity={activity} entityType="deal" entityId="d-1" />,
    );
    expect(screen.queryByRole("button", { name: "Undo filing" })).toBeNull();
  });
});
