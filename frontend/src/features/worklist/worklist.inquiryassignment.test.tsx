/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  INQUIRY_COORDINATOR,
  INQUIRY_FIRST_SOURCE,
  INQUIRY_MEMBERS,
} from "../../components/websiteinquiry.fixtures";
import { meFixture } from "../../testing/mefixture";
import { renderWithQueryClient } from "../../testing/queryrender";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { row } from "../../testing/worklist.testkit";
import {
  InquiryAssignmentDialog,
  InquiryTakeButton,
} from "./worklist.inquiryassignment";

const me = meFixture({
  allow: { activity: ["read", "update"], lead: ["read", "update"] },
});
const task: components["schemas"]["Activity"] = {
  ...INQUIRY_FIRST_SOURCE,
  id: "inquiry-task",
  kind: "task",
  subject: "Answer the website question",
  version: 7,
  source_system: "website_inquiry",
  source_id: "accepted-request",
  source_activity_id: INQUIRY_FIRST_SOURCE.id,
  web_form: undefined,
  is_done: false,
  assignee_id: "coordinator",
};
const item = row({
  id: task.id,
  source: "task",
  kind: "website_inquiry",
  version: 3,
  actions: ["take"],
  title: task.subject ?? undefined,
});
const path = `/activities/${task.id}/inquiry-assignment`;
function assignment(
  kind: "lead" | "contact",
  version = 11,
): components["schemas"]["WebsiteInquiryAssignment"] {
  return {
    task: { ...task, version: version - 4 },
    record_type: kind,
    record_id: "selected-record",
    record_version: version,
    record_owner_id: kind === "lead" ? null : "record-owner",
    recipient_id: me.user.id,
    changes_record_owner: kind === "lead",
  };
}
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const recordKinds: ReadonlyArray<"lead" | "contact"> = ["lead", "contact"];
it.each(recordKinds)(
  "reviews the %s and sends its exact identity and both versions",
  async (kind) => {
    const write = vi.fn(() => jsonResponse(assignment(kind)));
    installFetchStub({
      "GET /me": () => jsonResponse(me),
      [`POST ${path}/preview`]: () => jsonResponse(assignment(kind)),
      [`POST ${path}`]: write,
    });
    const fetch = vi.spyOn(globalThis, "fetch");
    const user = userEvent.setup();
    renderWithQueryClient(<InquiryTakeButton item={item} />);
    await user.click(screen.getByRole("button", { name: "Take inquiry" }));
    const dialog = await screen.findByRole("dialog", { name: "Take inquiry" });
    await within(dialog).findByText(task.subject ?? "");
    expect(write).not.toHaveBeenCalled();
    if (kind === "contact")
      expect(
        within(dialog).getByText(
          "This changes the inquiry task holder. The record keeps its owner.",
        ),
      ).toBeVisible();
    await user.click(
      within(dialog).getByRole("button", { name: "Take inquiry" }),
    );
    await waitFor(() =>
      expect(write).toHaveBeenCalledWith({
        mode: "take",
        recipient_id: me.user.id,
        record_type: kind,
        record_id: "selected-record",
        record_version: 11,
      }),
    );
    const requests = fetch.mock.calls
      .map(([input]) => input)
      .filter(
        (input): input is Request =>
          input instanceof Request &&
          input.method === "POST" &&
          input.url.endsWith(path),
      );
    expect(requests).toHaveLength(1);
    expect(requests[0].headers.get("If-Match")).toBe("7");
  },
);

it("requires a fresh review after a stale assignment is refused", async () => {
  let reviews = 0;
  const preview = vi.fn(() =>
    jsonResponse(assignment("lead", ++reviews === 1 ? 11 : 12)),
  );
  const write = vi
    .fn()
    .mockImplementationOnce(() =>
      jsonResponse(
        {
          status: 409,
          code: "version_skew",
          detail: "The inquiry changed. Review it again.",
        },
        409,
      ),
    )
    .mockImplementationOnce(() => jsonResponse(assignment("lead", 12)));
  installFetchStub({
    "GET /me": () => jsonResponse(me),
    [`POST ${path}/preview`]: preview,
    [`POST ${path}`]: write,
  });
  const fetch = vi.spyOn(globalThis, "fetch");
  const user = userEvent.setup();
  renderWithQueryClient(<InquiryTakeButton item={item} />);
  await user.click(screen.getByRole("button", { name: "Take inquiry" }));
  const dialog = await screen.findByRole("dialog", { name: "Take inquiry" });
  await within(dialog).findByText(task.subject ?? "");
  await user.click(
    within(dialog).getByRole("button", { name: "Take inquiry" }),
  );
  const review = await within(dialog).findByRole("button", {
    name: "Review current assignment",
  });
  expect(
    within(dialog).getByRole("button", { name: "Take inquiry" }),
  ).toBeDisabled();
  await user.click(review);
  await waitFor(() =>
    expect(
      within(dialog).getByRole("button", { name: "Take inquiry" }),
    ).toBeEnabled(),
  );
  await user.click(
    within(dialog).getByRole("button", { name: "Take inquiry" }),
  );
  await waitFor(() =>
    expect(write).toHaveBeenLastCalledWith({
      mode: "take",
      recipient_id: me.user.id,
      record_type: "lead",
      record_id: "selected-record",
      record_version: 12,
    }),
  );
  const requests = fetch.mock.calls
    .map(([input]) => input)
    .filter(
      (input): input is Request =>
        input instanceof Request &&
        input.method === "POST" &&
        input.url.endsWith(path),
    );
  expect(requests.map((request) => request.headers.get("If-Match"))).toEqual([
    "7",
    "8",
  ]);
});

it("reviews the selected colleague before handing over the lead and its inquiry task", async () => {
  const plan = { ...assignment("lead"), recipient_id: INQUIRY_COORDINATOR };
  const preview = vi.fn(() => jsonResponse(plan));
  const write = vi.fn(() => jsonResponse(plan));
  const close = vi.fn();
  const members = () =>
    jsonResponse({
      data: INQUIRY_MEMBERS,
      page: { has_more: false, next_cursor: null },
    });
  installFetchStub({
    "GET /me": () => jsonResponse(me),
    "GET /users": members,
    "GET /users/names": members,
    [`POST ${path}/preview`]: preview,
    [`POST ${path}`]: write,
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <InquiryAssignmentDialog
      item={item}
      mode="assign_and_follow_up"
      onClose={close}
    />,
  );
  await user.click(
    await screen.findByRole("combobox", { name: "Choose a colleague" }),
  );
  await user.click(
    await screen.findByRole("option", { name: "Mina Coordinator" }),
  );
  await screen.findByText(
    "The lead and this inquiry task go to this colleague.",
  );
  expect(preview).toHaveBeenCalledWith({
    mode: "assign_and_follow_up",
    recipient_id: INQUIRY_COORDINATOR,
  });
  expect(write).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "Assign and follow up" }),
  );
  await waitFor(() =>
    expect(write).toHaveBeenCalledWith({
      mode: "assign_and_follow_up",
      recipient_id: INQUIRY_COORDINATOR,
      record_type: "lead",
      record_id: "selected-record",
      record_version: 11,
    }),
  );
  await waitFor(() => expect(close).toHaveBeenCalled());
});

it("refuses a record type this client cannot review", async () => {
  const write = vi.fn(() => jsonResponse(assignment("lead")));
  installFetchStub({
    "GET /me": () => jsonResponse(me),
    [`POST ${path}/preview`]: () =>
      jsonResponse({ ...assignment("lead"), record_type: "future_record" }),
    [`POST ${path}`]: write,
  });
  const user = userEvent.setup();
  renderWithQueryClient(<InquiryTakeButton item={item} />);
  await user.click(screen.getByRole("button", { name: "Take inquiry" }));
  const dialog = await screen.findByRole("dialog", { name: "Take inquiry" });
  expect(
    await within(dialog).findByText(
      "This version cannot assign this inquiry. Reload the page.",
    ),
  ).toBeVisible();
  expect(
    within(dialog).getByRole("button", { name: "Take inquiry" }),
  ).toBeDisabled();
  expect(write).not.toHaveBeenCalled();
});
