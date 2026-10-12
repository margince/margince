/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TaskQuickActions, useTaskUpdate } from "./taskactions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Row() {
  const update = useTaskUpdate([]);
  return <TaskQuickActions activityId="task-1" version={2} update={update} />;
}

describe("the Done verb beside a task", () => {
  it("confirms with an Undo that reopens the task on the version the write produced", async () => {
    const writes: { body: unknown; ifMatch: string | null }[] = [];
    installFetchStub({});
    const routed = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.method === "PATCH") {
          writes.push({
            body: await request.clone().json(),
            ifMatch: request.headers.get("If-Match"),
          });
          return jsonResponse({ id: "task-1", version: 7 });
        }
        return routed(input, init);
      }),
    );
    render(
      <StoryProviders>
        <ToastProvider>
          <Row />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>,
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(await screen.findByText("Task completed")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[0].body).toEqual({ is_done: true });
    expect(writes[1].body).toEqual({ is_done: false });
    expect(writes[1].ifMatch).toContain("7");
  });
});
