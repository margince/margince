// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { captureReview, captureReviewLink } from "./capturereview";

it("copies the personal review address without query credentials or another reader identity", () => {
  expect(
    captureReviewLink(
      "senders",
      "https://app.example.test/crm/?code=private-token#/old",
    ),
  ).toBe("https://app.example.test/crm/#/home?queue=1&queue_view=decisions");
  expect(captureReview("held")).toBe("held");
  expect(captureReview("senders")).toBeNull();
  expect(captureReview("https://external.example.test")).toBeNull();
});
