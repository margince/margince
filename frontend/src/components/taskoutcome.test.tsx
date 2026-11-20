// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import {
  type Captured,
  createdActivity,
  jsonResponse,
  render,
  stubApi,
} from "./logactivity.testkit";
import { TaskOutcomeForm } from "./taskoutcome";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const unfiled: components["schemas"]["Activity"] = {
  id: "t1",
  kind: "task",
  subject: "Renew the office insurance",
  occurred_at: "2026-10-01T09:00:00Z",
  occurred_precision: "instant",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-10-01T09:00:00Z",
  updated_at: "2026-10-01T09:00:00Z",
  offers_email: false,
  is_done: false,
  version: 1,
};

// A task filed on no record still has an outcome, and an activity needs no
// record. So the form takes "none" as an answer, and keeps a way back.
describe("the outcome of a task filed on no record", () => {
  it("logs the outcome with no record", async () => {
    const user = userEvent.setup();
    const captured: Captured[] = [];
    stubApi(
      {
        "POST /activities": createdActivity,
        "PATCH /activities/t1": () => jsonResponse({ ...unfiled, version: 2 }),
      },
      captured,
    );
    render(
      <TaskOutcomeForm task={unfiled} onLogged={() => {}} onBack={() => {}} />,
    );

    await user.click(
      screen.getByRole("button", { name: "Log without a record" }),
    );
    await user.type(await screen.findByLabelText("Subject *"), "Renewed");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(captured.some((entry) => entry.key === "POST /activities")).toBe(
        true,
      ),
    );
    const post = captured.find((entry) => entry.key === "POST /activities");
    expect(post?.body).toMatchObject({
      kind: "note",
      subject: "Renewed",
      source_activity_id: "t1",
      links: [],
    });
  });

  it("holds Back while the outcome is being logged", async () => {
    const user = userEvent.setup();
    stubApi({ "POST /activities": () => new Promise<Response>(() => {}) });
    render(
      <TaskOutcomeForm task={unfiled} onLogged={() => {}} onBack={() => {}} />,
    );

    await user.click(
      screen.getByRole("button", { name: "Log without a record" }),
    );
    await user.type(await screen.findByLabelText("Subject *"), "Renewed");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Back" })).toHaveProperty(
        "disabled",
        true,
      ),
    );
  });

  it("keeps Back held once the note exists and only the task is left open", async () => {
    const user = userEvent.setup();
    const captured: Captured[] = [];
    stubApi(
      {
        "POST /activities": createdActivity,
        "GET /activities/t1": () => jsonResponse(unfiled),
        "PATCH /activities/t1": () =>
          jsonResponse({ title: "unavailable", status: 503 }, 503),
      },
      captured,
    );
    render(
      <TaskOutcomeForm task={unfiled} onLogged={() => {}} onBack={() => {}} />,
    );

    await user.click(
      screen.getByRole("button", { name: "Log without a record" }),
    );
    await user.type(await screen.findByLabelText("Subject *"), "Renewed");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await screen.findByText(
      /The activity is logged and can no longer be changed/,
    );
    expect(screen.getByRole("button", { name: "Back" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(
      captured.filter((entry) => entry.key === "POST /activities"),
    ).toHaveLength(1);
  });

  it("retries what was sent when the subject was cleared mid-request", async () => {
    const user = userEvent.setup();
    const captured: Captured[] = [];
    let created: (answer: Response) => void = () => {};
    let completions = 0;
    const onLogged = vi.fn();
    stubApi(
      {
        "POST /activities": (body) =>
          new Promise<Response>((resolve) => {
            created = () => resolve(createdActivity(body));
          }),
        "GET /activities/t1": () => jsonResponse(unfiled),
        "PATCH /activities/t1": () =>
          ++completions === 1
            ? jsonResponse({ title: "unavailable", status: 503 }, 503)
            : jsonResponse({ ...unfiled, is_done: true, version: 2 }),
      },
      captured,
    );
    render(
      <TaskOutcomeForm task={unfiled} onLogged={onLogged} onBack={() => {}} />,
    );

    await user.click(
      screen.getByRole("button", { name: "Log without a record" }),
    );
    const subject = await screen.findByLabelText("Subject *");
    await user.type(subject, "Renewed");
    await user.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() =>
      expect(captured.some((entry) => entry.key === "POST /activities")).toBe(
        true,
      ),
    );
    await user.clear(subject);
    created(new Response());
    await screen.findByText(
      /The activity is logged and can no longer be changed/,
    );

    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() => expect(onLogged).toHaveBeenCalledTimes(1));
    expect(completions).toBe(2);
    expect(
      captured.filter((entry) => entry.key === "POST /activities"),
    ).toHaveLength(1);
  });

  it("goes back to the task's actions", async () => {
    const user = userEvent.setup();
    stubApi({});
    const onBack = vi.fn();
    render(
      <TaskOutcomeForm task={unfiled} onLogged={() => {}} onBack={onBack} />,
    );

    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(onBack).toHaveBeenCalledTimes(1);
  });
});
