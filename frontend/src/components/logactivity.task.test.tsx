/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { dueInstant } from "../format/calendarday";
import { LogActivity } from "./logactivity";
import {
  type Captured,
  contact,
  createdActivity,
  INSTALLATION_ZONE,
  jsonResponse,
  render,
  stubApi,
} from "./logactivity.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("assigning a task at create time", () => {
  // Two contacts and an agent seat, so the picker's eligibility is a real filter
  // rather than an empty list agreeing with itself. The agent is who the server
  // refuses as an assignee, so it is the one the picker must not offer.
  const rosterUsers = {
    data: [
      { id: "u1", display_name: "Dana Ops", is_agent: false },
      { id: "u2", display_name: "Priya Lead", is_agent: false },
      { id: "agent-1", display_name: "Runner Bot", is_agent: true },
    ],
    page: { next_cursor: null },
  };

  function renderTaskComposer(captured?: Captured[]) {
    stubApi(
      {
        "GET /contacts/p1": () => jsonResponse(contact),
        "GET /users": () => jsonResponse(rosterUsers),
        "POST /activities": createdActivity,
      },
      captured,
    );
    render(<LogActivity entityType="contact" entityId="p1" askedKind="task" />);
  }

  it("carries the chosen assignee on the posted task", async () => {
    const captured: Captured[] = [];
    renderTaskComposer(captured);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Subject *"), "Send proposal");
    await user.click(screen.getByLabelText("Assignee"));
    await user.click(await screen.findByRole("option", { name: "Priya Lead" }));
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(captured.some((e) => e.key === "POST /activities")).toBe(true),
    );
    const post = captured.find((e) => e.key === "POST /activities");
    expect(post?.body).toMatchObject({ kind: "task", assignee_id: "u2" });
  });

  it("posts no assignee when the task is left unassigned", async () => {
    const captured: Captured[] = [];
    renderTaskComposer(captured);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Subject *"), "Send proposal");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(captured.some((e) => e.key === "POST /activities")).toBe(true),
    );
    const post = captured.find((e) => e.key === "POST /activities");
    expect(post?.body).toMatchObject({ kind: "task" });
    expect(post?.body).not.toHaveProperty("assignee_id");
  });

  it("offers no assignee on a note, which has nobody to hold it", async () => {
    stubApi({
      "GET /contacts/p1": () => jsonResponse(contact),
      "GET /users": () => jsonResponse(rosterUsers),
    });
    render(<LogActivity entityType="contact" entityId="p1" />);
    await screen.findByLabelText("Subject *");
    expect(screen.queryByLabelText("Assignee")).toBeNull();
  });

  it("posts a task's callback time as its due time, flagged as a time", async () => {
    const captured: Captured[] = [];
    renderTaskComposer(captured);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Subject *"), "Call back");
    const time = screen.getByRole("textbox", { name: "Time (optional)" });
    await user.tripleClick(screen.getByRole("textbox", { name: "Due date" }));
    await user.keyboard("27/10/2027");
    await user.tab();
    await user.type(time, "15:00");
    await user.tab();
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() =>
      expect(captured.some((e) => e.key === "POST /activities")).toBe(true),
    );
    const post = captured.find((e) => e.key === "POST /activities");
    const due = dueInstant("2027-10-27", INSTALLATION_ZONE, "15:00");
    expect(post?.body).toMatchObject({
      kind: "task",
      due_at: due,
      due_has_time: true,
    });
    expect(post?.body).not.toHaveProperty("remind_at");
  });

  it("offers no callback time on a note", async () => {
    stubApi({
      "GET /contacts/p1": () => jsonResponse(contact),
      "GET /users": () => jsonResponse(rosterUsers),
    });
    render(<LogActivity entityType="contact" entityId="p1" />);
    await screen.findByLabelText("Subject *");
    expect(screen.queryByLabelText("Time (optional)")).toBeNull();
  });

  it("does not offer an agent seat as an assignee", async () => {
    renderTaskComposer();
    const user = userEvent.setup();
    await user.click(await screen.findByLabelText("Assignee"));
    const listbox = screen.getByRole("listbox");
    expect(
      await within(listbox).findByRole("option", { name: "Dana Ops" }),
    ).toBeTruthy();
    expect(
      within(listbox).queryByRole("option", { name: "Runner Bot" }),
    ).toBeNull();
  });
});
