/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  INQUIRY_FIRST_SOURCE,
  INQUIRY_GRANTS,
} from "../../components/websiteinquiry.fixtures";
import { OpenSourceProvider } from "../../lib/opensource";
import { renderWithQueryClient } from "../../testing/queryrender";
import { installFetchStub, meRoute } from "../../testing/story-utils";
import { WebsiteInquiryFacts } from "./websiteinquiryfacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("shows accepted facts and opens their actual submission", async () => {
  installFetchStub({ "GET /me": meRoute(INQUIRY_GRANTS) });
  const open = vi.fn();
  const user = userEvent.setup();
  renderWithQueryClient(
    <OpenSourceProvider openSource={open}>
      <WebsiteInquiryFacts
        facts={{
          received_at: INQUIRY_FIRST_SOURCE.created_at,
          source_activity_id: INQUIRY_FIRST_SOURCE.id,
          source_label: "Website contact",
          request_type: "Quote",
          assignment_reason: "coordinator_review",
        }}
      />
    </OpenSourceProvider>,
  );
  expect(screen.getByText("Request type: Quote")).toBeVisible();
  expect(
    screen.getByText("Initial handling: Coordinator reviews first"),
  ).toBeVisible();
  expect(screen.getByText(/^Received:/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Open submission" }));
  expect(open).toHaveBeenCalledWith(INQUIRY_FIRST_SOURCE.id);
});

it("keeps unavailable source facts unknown", () => {
  installFetchStub({ "GET /me": meRoute(INQUIRY_GRANTS) });
  renderWithQueryClient(<WebsiteInquiryFacts facts={{}} />);
  expect(screen.getByText("Receipt details unavailable")).toBeVisible();
  expect(screen.getByText("Request type: Not specified")).toBeVisible();
  expect(
    screen.getByText("Initial handling: Reason not recorded"),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Open submission" }),
  ).not.toBeInTheDocument();
});
