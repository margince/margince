/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
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

function stubs(
  preview: () => Response,
  execute: (body: unknown) => Response,
  sources: readonly object[] = [first, second],
) {
  installFetchStub({
    "GET /me": meRoute(INQUIRY_GRANTS),
    "GET /activities": () =>
      jsonResponse({
        data: sources,
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
    await screen.findByRole("checkbox", { name: "Select Lena Vogel" }),
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Select ops@example.test" }),
  );
  expect(preview).not.toHaveBeenCalled();
  expect(write).not.toHaveBeenCalled();
  await user.click(
    screen.getByRole("button", { name: "Review selected inquiries" }),
  );
  expect(await screen.findByText("Ready to handle")).toBeVisible();
  expect(screen.getByText("Already handled")).toBeVisible();
  expect(screen.getByText("Lena Vogel")).toBeVisible();
  expect(screen.getByText("ops@example.test")).toBeVisible();
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
    expect.any(URL),
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
    await screen.findByRole("checkbox", { name: "Select Lena Vogel" }),
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
      screen.getByRole("checkbox", { name: "Select Lena Vogel" }),
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
    await screen.findByRole("checkbox", { name: "Select Lena Vogel" }),
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

it("tells submissions of one form apart by who sent them and how their message opens", async () => {
  const unkept = {
    ...first,
    id: "00000000-0000-4000-8000-000000000024",
    subject: "Website form: Website contact",
    web_form: null,
  };
  const blank = {
    ...first,
    id: "00000000-0000-4000-8000-000000000025",
    web_form: {
      full_name: "   ",
      email: "jo@example.test",
      message: "A second question.",
    },
  };
  stubs(
    () => jsonResponse({ items: [] }),
    () => jsonResponse({ items: [] }),
    [first, second, unkept, blank],
  );
  renderWithQueryClient(
    <WebsiteInquiryExistingDialog
      form={INQUIRY_FORM}
      automation={INQUIRY_AUTOMATION}
      onClose={() => {}}
    />,
  );
  const named = (
    await screen.findByRole("checkbox", { name: "Select Lena Vogel" })
  ).closest("tr");
  if (!named) throw new Error("the row of a named sender is missing");
  expect(within(named).getByText("lena@example.test")).toBeVisible();
  expect(
    within(named).getByText("Please call me about a quote."),
  ).toBeVisible();
  expect(within(named).queryByText(/We need it by Friday/)).toBeNull();
  expect(within(named).queryByText("Website form: Website contact")).toBeNull();
  // A sender who gave no name is named by their address, shown once.
  const unnamed = screen
    .getByRole("checkbox", { name: "Select ops@example.test" })
    .closest("tr");
  if (!unnamed) throw new Error("the row of an unnamed sender is missing");
  expect(within(unnamed).getAllByText("ops@example.test")).toHaveLength(1);
  expect(within(unnamed).getByText("Our order has not arrived.")).toBeVisible();
  // A name of spaces is no name: the row reads by the address, not blank.
  expect(
    screen.getByRole("checkbox", { name: "Select jo@example.test" }),
  ).toBeInTheDocument();
  // A note whose fields were never kept still reads by its subject.
  expect(
    screen.getByRole("checkbox", {
      name: "Select Website form: Website contact",
    }),
  ).toBeInTheDocument();
});
