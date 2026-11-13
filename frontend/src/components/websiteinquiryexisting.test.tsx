/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { renderWithQueryClient } from "../testing/queryrender";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../testing/story-utils";
import {
  INQUIRY_FIRST_SOURCE as first,
  INQUIRY_AUTOMATION,
  INQUIRY_COORDINATOR,
  INQUIRY_FORM,
  INQUIRY_GRANTS,
  INQUIRY_MEMBERS,
  INQUIRY_SECOND_SOURCE as second,
} from "./websiteinquiry.fixtures";
import { WebsiteInquiryExistingDialog } from "./websiteinquiryexisting";

const path = `/web-forms/${INQUIRY_FORM.id}/handling`;
const ready = {
  id: first.id,
  version: first.version,
  status: "eligible",
  recipient_id: INQUIRY_COORDINATOR,
  due_at: "2026-09-02T21:59:59.999999Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubs(preview: () => Response, execute: (body: unknown) => Response) {
  installFetchStub({
    "GET /me": meRoute(INQUIRY_GRANTS),
    "GET /activities": () =>
      jsonResponse({
        data: [first, second],
        page: { has_more: false, next_cursor: null },
      }),
    "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    [`POST ${path}/preview`]: preview,
    [`POST ${path}/existing`]: execute,
  });
}

it("reviews current versions and handles only eligible submissions without enabling future handling", async () => {
  const write = vi.fn(() =>
    jsonResponse({
      items: [
        {
          id: first.id,
          version: 4,
          status: "task_created",
          task_id: "00000000-0000-4000-8000-000000000023",
        },
      ],
    }),
  );
  const preview = vi.fn(() =>
    jsonResponse({
      received_at: first.created_at,
      example_due_at: ready.due_at,
      items: [
        ready,
        { id: second.id, version: second.version, status: "already_handled" },
      ],
    }),
  );
  stubs(preview, write);
  const close = vi.fn();
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryExistingDialog
      form={INQUIRY_FORM}
      automation={INQUIRY_AUTOMATION}
      onClose={close}
    />,
  );
  await user.click(
    await screen.findByRole("checkbox", { name: "Select A website question" }),
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Select An older question" }),
  );
  expect(preview).not.toHaveBeenCalled();
  expect(write).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "Review selected inquiries" }),
  );
  expect(await screen.findByText("Ready to handle")).toBeVisible();
  expect(screen.getByText("Already handled")).toBeVisible();
  expect(write).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "Create the reviewed tasks" }),
  );
  expect(await screen.findByText("Task created")).toBeVisible();
  expect(write).toHaveBeenCalledWith(
    expect.objectContaining({
      automation_id: INQUIRY_AUTOMATION.id,
      automation_version: INQUIRY_AUTOMATION.version,
      selected: [{ id: first.id, version: first.version }],
    }),
  );
  expect(close).not.toHaveBeenCalled();
  expect(
    screen.queryByRole("button", { name: "Create the reviewed tasks" }),
  ).not.toBeInTheDocument();
});

it("keeps a changed result visible so it can be read and reviewed again", async () => {
  const write = vi.fn(() =>
    jsonResponse({
      items: [{ id: first.id, version: first.version, status: "changed" }],
    }),
  );
  stubs(
    () =>
      jsonResponse({
        received_at: first.created_at,
        example_due_at: ready.due_at,
        items: [ready],
      }),
    write,
  );
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryExistingDialog
      form={INQUIRY_FORM}
      automation={INQUIRY_AUTOMATION}
      onClose={() => {}}
    />,
  );
  await user.click(
    await screen.findByRole("checkbox", { name: "Select A website question" }),
  );
  await user.click(
    screen.getByRole("button", { name: "Review selected inquiries" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Create the reviewed tasks" }),
  );
  expect(
    await screen.findByText("Changed since selection. Read it again."),
  ).toBeVisible();
  await user.click(
    screen.getByRole("button", { name: "Choose submissions again" }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("checkbox", { name: "Select A website question" }),
    ).not.toBeChecked(),
  );
  expect(
    screen.getByRole("button", { name: "Review selected inquiries" }),
  ).toBeDisabled();
});

it("refuses to create tasks when the selected-source preview fails", async () => {
  const write = vi.fn(() => jsonResponse({ items: [] }));
  stubs(
    () =>
      jsonResponse(
        {
          code: "permission_denied",
          status: 403,
          detail: "You cannot read these submissions.",
        },
        403,
      ),
    write,
  );
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryExistingDialog
      form={INQUIRY_FORM}
      automation={INQUIRY_AUTOMATION}
      onClose={() => {}}
    />,
  );
  await user.click(
    await screen.findByRole("checkbox", { name: "Select A website question" }),
  );
  await user.click(
    screen.getByRole("button", { name: "Review selected inquiries" }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Create the reviewed tasks" }),
    ).toBeDisabled(),
  );
  expect(write).not.toHaveBeenCalled();
});
