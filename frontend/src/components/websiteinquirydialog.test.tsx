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
  INQUIRY_AUTOMATION,
  INQUIRY_DEADLINE_PREVIEW,
  INQUIRY_FORM,
  INQUIRY_GRANTS,
  INQUIRY_MEMBERS,
  INQUIRY_POLICY,
  INQUIRY_REQUEST_FIELD,
} from "./websiteinquiry.fixtures";
import { WebsiteInquiryDialog } from "./websiteinquirydialog";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

it("reviews the shared policy before saving it and keeps authority separate from the task recipient", async () => {
  const write = vi.fn((body: unknown) =>
    jsonResponse({
      form: INQUIRY_FORM,
      automation: INQUIRY_AUTOMATION,
      received: body,
    }),
  );
  installFetchStub({
    "GET /me": meRoute(INQUIRY_GRANTS),
    [`POST /web-forms/${INQUIRY_FORM.id}/handling/preview`]: () =>
      jsonResponse(INQUIRY_DEADLINE_PREVIEW),
    "GET /custom-fields": () =>
      jsonResponse({
        data: [
          INQUIRY_REQUEST_FIELD,
          {
            ...INQUIRY_REQUEST_FIELD,
            id: "00000000-0000-4000-8000-000000000006",
            column_name: "cf_notes",
            label: "Notes",
            type: "text",
          },
        ],
      }),
    "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    [`GET /automations/${INQUIRY_AUTOMATION.id}`]: () =>
      jsonResponse(INQUIRY_AUTOMATION),
    [`PUT /web-forms/${INQUIRY_FORM.id}/handling`]: write,
  });
  const installed = globalThis.fetch;
  let reviewedVersion: string | null = null;
  globalThis.fetch = async (input, init) => {
    if (input instanceof Request && input.method === "PUT")
      reviewedVersion = input.headers.get("If-Match");
    return installed(input, init);
  };
  const closed = vi.fn();
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryDialog form={INQUIRY_FORM} onClose={closed} />,
  );
  const title = await screen.findByRole("textbox", { name: "Task title" });
  expect(title).toHaveValue(INQUIRY_POLICY.task_title);
  const requestType = await screen.findByRole("combobox", {
    name: "Request type field (optional)",
  });
  await waitFor(() => expect(requestType).toBeEnabled());
  await user.click(requestType);
  expect(
    screen.queryByRole("option", { name: "Notes" }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: "Request type" }));
  await user.clear(title);
  await user.type(title, "Review this new inquiry");
  expect(write).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Review handling" }));
  expect(screen.getByText("Authorized by")).toBeVisible();
  await waitFor(() =>
    expect(
      screen.getByText("Authorized by").nextElementSibling,
    ).toHaveTextContent("Test User"),
  );
  await waitFor(() =>
    expect(
      screen.getByText("Coordinator").nextElementSibling,
    ).toHaveTextContent("Mina Coordinator"),
  );
  expect(write).not.toHaveBeenCalled();
  expect(await screen.findByText(/^A question received at /)).toBeVisible();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Save paused" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "Save paused" }));
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(reviewedVersion).toBe(String(INQUIRY_FORM.version));
  expect(write.mock.calls[0]?.[0]).toMatchObject({
    name: INQUIRY_AUTOMATION.name,
    automation_id: INQUIRY_AUTOMATION.id,
    automation_version: 7,
    enabled: false,
    policy: {
      ...INQUIRY_POLICY,
      request_type_field: "cf_request_type",
      task_title: "Review this new inquiry",
    },
  });
  await waitFor(() => expect(closed).toHaveBeenCalledOnce());
});

it("keeps the proposed policy visible when a stale review is refused", async () => {
  installFetchStub({
    "GET /me": meRoute(INQUIRY_GRANTS),
    [`POST /web-forms/${INQUIRY_FORM.id}/handling/preview`]: () =>
      jsonResponse(INQUIRY_DEADLINE_PREVIEW),
    "GET /custom-fields": () => jsonResponse({ data: [] }),
    "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    [`GET /automations/${INQUIRY_AUTOMATION.id}`]: () =>
      jsonResponse(INQUIRY_AUTOMATION),
    [`PUT /web-forms/${INQUIRY_FORM.id}/handling`]: () =>
      jsonResponse(
        {
          code: "version_skew",
          detail: "The form changed after this review.",
          status: 409,
        },
        409,
      ),
  });
  const closed = vi.fn();
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryDialog form={INQUIRY_FORM} onClose={closed} />,
  );
  await screen.findByRole("textbox", { name: "Task title" });
  await user.click(screen.getByRole("button", { name: "Review handling" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Turn on this handling" }),
    ).toBeEnabled(),
  );
  await user.click(
    screen.getByRole("button", { name: "Turn on this handling" }),
  );
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(closed).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Back to settings" }));
  expect(screen.getByRole("textbox", { name: "Task title" })).toHaveValue(
    INQUIRY_POLICY.task_title,
  );
});

it("keeps activation unavailable when the current deadline preview fails", async () => {
  installFetchStub({
    "GET /me": meRoute(INQUIRY_GRANTS),
    "GET /custom-fields": () => jsonResponse({ data: [] }),
    "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    [`GET /automations/${INQUIRY_AUTOMATION.id}`]: () =>
      jsonResponse(INQUIRY_AUTOMATION),
    [`POST /web-forms/${INQUIRY_FORM.id}/handling/preview`]: () =>
      jsonResponse(
        {
          code: "version_skew",
          detail: "The policy changed after it was opened.",
          status: 409,
        },
        409,
      ),
  });
  const user = userEvent.setup();
  renderWithQueryClient(
    <WebsiteInquiryDialog form={INQUIRY_FORM} onClose={() => {}} />,
  );
  await screen.findByRole("textbox", { name: "Task title" });
  await user.click(screen.getByRole("button", { name: "Review handling" }));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(
    screen.getByRole("button", { name: "Turn on this handling" }),
  ).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Back to settings" }));
  expect(screen.getByRole("textbox", { name: "Task title" })).toHaveValue(
    INQUIRY_POLICY.task_title,
  );
});
