// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
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

const project = {
  id: "proj-1",
  name: "Pallet Handling Programme",
  version: 5,
  owner_id: null,
} as unknown as Project;

type Recorded = {
  url: string;
  method: string;
  body: unknown;
  ifMatch: string | null;
};

function stubApi(
  response: { body: Record<string, unknown>; status?: number },
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
          ],
          page: { has_more: false },
        });
      }
      if (url.includes("/projects/proj-1") && method === "PATCH") {
        const rawBody = request ? await request.text() : (init?.body ?? null);
        const body = rawBody ? JSON.parse(String(rawBody)) : null;
        const headers = request ? request.headers : new Headers(init?.headers);
        calls.push({ url, method, body, ifMatch: headers.get("If-Match") });
        return jsonResponse(response.body, response.status ?? 200);
      }
      return jsonResponse({ data: [], page: { has_more: false } });
    }),
  );
}

describe("AssignProjectOwnerAction", () => {
  it("searches, picks a named colleague, and PATCHes owner_id with the current version as If-Match", async () => {
    const calls: Recorded[] = [];
    stubApi({ body: { ...project, owner_id: "u-42", version: 6 } }, calls);
    const user = userEvent.setup();
    const { client } = render(<AssignProjectOwnerAction project={project} />);
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");

    await user.click(screen.getByTestId("assign-project-owner"));
    await user.type(
      screen.getByRole("searchbox", { name: "Search colleagues" }),
      "Jane",
    );
    await user.click(await screen.findByRole("button", { name: "Jane Doe" }));
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Confirm" })).toBeNull(),
    );
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

  it("refuses to confirm before a colleague is picked", async () => {
    const calls: Recorded[] = [];
    stubApi({ body: { ...project, owner_id: "u-42", version: 6 } }, calls);
    const user = userEvent.setup();
    render(<AssignProjectOwnerAction project={project} />);

    await user.click(screen.getByTestId("assign-project-owner"));
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    expect(calls).toHaveLength(0);
  });

  it("renders a 409 conflict detail verbatim rather than failing silently", async () => {
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

    await user.click(screen.getByTestId("assign-project-owner"));
    await user.type(
      screen.getByRole("searchbox", { name: "Search colleagues" }),
      "Jane",
    );
    await user.click(await screen.findByRole("button", { name: "Jane Doe" }));
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(
      await screen.findByText("the project was changed by someone else"),
    ).toBeTruthy();
    // The dialog stays open on failure — the reader can retry or cancel,
    // rather than the write vanishing with nothing on screen.
    expect(screen.getByRole("button", { name: "Confirm" })).toBeTruthy();

    // Closing and reopening starts clean: a failure from an attempt nobody
    // has repeated yet must not resurface as if it just happened again.
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByTestId("assign-project-owner"));
    expect(
      screen.queryByText("the project was changed by someone else"),
    ).toBeNull();
  });
});
