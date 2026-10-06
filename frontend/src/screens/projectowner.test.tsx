// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { AssignProjectOwnerAction } from "./projectowner";
import type { Project } from "./projects.form";

// The one path that hands a project directly to a NAMED colleague from the
// project's own screen, backed by the server's existing owner_id field
// (updateProject) rather than the bulk, human-only transfer endpoint.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const result = rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...result, client };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const project: Project = {
  id: "proj-1",
  name: "Pallet Handling Programme",
  phase: "delivering",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  version: 5,
  owner_id: null,
};

type Recorded = {
  url: string;
  method: string;
  body: unknown;
  ifMatch: string | null;
};

function stubApi(
  response: {
    body: Record<string, unknown>;
    status?: number;
    /** Holds the PATCH's answer until it settles. */
    held?: Promise<void>;
  },
  calls: Recorded[],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.includes("/users")) {
        return jsonResponse({
          data: [
            {
              id: "u-42",
              display_name: "Jane Doe",
              email: "jane@example.test",
            },
            {
              id: "u-7",
              display_name: "Omar Haddad",
              email: "omar@example.test",
            },
          ],
          page: { has_more: false },
        });
      }
      if (url.includes("/projects/proj-1") && method === "PATCH") {
        const rawBody = request ? await request.text() : (init?.body ?? null);
        const body = rawBody ? JSON.parse(String(rawBody)) : null;
        const headers = request ? request.headers : new Headers(init?.headers);
        calls.push({ url, method, body, ifMatch: headers.get("If-Match") });
        await response.held;
        return jsonResponse(response.body, response.status ?? 200);
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole("button", { name: "Assign to a colleague" }),
  );
  return within(await screen.findByRole("dialog"));
}

describe("AssignProjectOwnerAction", () => {
  it("opens on the roster, and a pick PATCHes owner_id with the current version as If-Match", async () => {
    const calls: Recorded[] = [];
    stubApi({ body: { ...project, owner_id: "u-42", version: 6 } }, calls);
    const user = userEvent.setup();
    const { client } = render(<AssignProjectOwnerAction project={project} />);
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toMatchObject({ owner_id: "u-42" });
    expect(calls[0].ifMatch).toBe("5");
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["projects"] }),
    );
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["project", "proj-1"] }),
    );
  });

  it("marks the current owner, and picking them again writes nothing", async () => {
    const calls: Recorded[] = [];
    stubApi({ body: project }, calls);
    const user = userEvent.setup();
    render(
      <AssignProjectOwnerAction project={{ ...project, owner_id: "u-7" }} />,
    );

    const panel = await openPicker(user);
    const current = await panel.findByRole("option", { name: "Omar Haddad" });
    expect(current).toHaveAttribute("aria-selected", "true");
    await user.click(current);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(calls).toHaveLength(0);
  });

  it("finds a colleague by email as well as by name", async () => {
    stubApi({ body: project }, []);
    const user = userEvent.setup();
    render(<AssignProjectOwnerAction project={project} />);

    const panel = await openPicker(user);
    await panel.findByRole("option", { name: "Jane Doe" });
    await user.type(panel.getByRole("combobox"), "omar@");

    expect(
      panel.getAllByRole("option").map((option) => option.textContent),
    ).toEqual(["Omar Haddad"]);
  });

  it("sends one PATCH for a double press on a colleague", async () => {
    const calls: Recorded[] = [];
    stubApi({ body: { ...project, owner_id: "u-42", version: 6 } }, calls);
    const user = userEvent.setup();
    render(<AssignProjectOwnerAction project={project} />);

    const panel = await openPicker(user);
    const option = await panel.findByRole("option", { name: "Jane Doe" });
    // One act holds both presses before any render, so `pending` is still
    // false for the second and only the latch can refuse it.
    act(() => {
      fireEvent.click(option);
      fireEvent.click(option);
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls).toHaveLength(1);
  });

  it("stays open through Escape while the PATCH is out, so a late refusal is read", async () => {
    const calls: Recorded[] = [];
    let release: () => void = () => {};
    stubApi(
      {
        body: { title: "Conflict", detail: "the project was changed" },
        status: 409,
        held: new Promise<void>((resolve) => {
          release = resolve;
        }),
      },
      calls,
    );
    const user = userEvent.setup();
    render(<AssignProjectOwnerAction project={project} />);

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));
    await waitFor(() => expect(calls).toHaveLength(1));

    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    release();
    expect(await panel.findByRole("alert")).toHaveTextContent(
      "the project was changed",
    );
  });

  it("keeps the list open over a 409, saying why, and reopens clean", async () => {
    const calls: Recorded[] = [];
    stubApi(
      {
        body: {
          title: "Conflict",
          detail: "the project was changed by someone else",
          code: "version_conflict",
        },
        status: 409,
      },
      calls,
    );
    const user = userEvent.setup();
    render(<AssignProjectOwnerAction project={project} />);

    const panel = await openPicker(user);
    await user.click(await panel.findByRole("option", { name: "Jane Doe" }));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(await panel.findByRole("alert")).toHaveTextContent(
      "the project was changed by someone else",
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    // A failure from an attempt nobody has repeated yet must not resurface
    // as if it just happened again.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: "Assign to a colleague" })).toBe(
      document.activeElement,
    );
    const reopened = await openPicker(user);
    await reopened.findByRole("option", { name: "Jane Doe" });
    expect(reopened.queryByRole("alert")).toBeNull();
  });
});
