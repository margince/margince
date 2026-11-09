// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import { deal } from "../../../testing/brief.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealActions } from "./dealactions";

type Deal = components["schemas"]["Deal"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function drawActions(over: Partial<Deal>) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"], activity: ["create"] }),
  });
  render(
    <StoryProviders>
      <DealActions
        deal={deal("d-1", "Fleet telematics rollout", over)}
        openStages={[]}
        onEditDetails={() => undefined}
      />
    </StoryProviders>,
  );
}

const meet = () =>
  screen.getByRole("button", { name: en["scheduling.bookContact"] });

describe("DealActions, Meet", () => {
  it("says why Meet is off on a deal with no company", async () => {
    drawActions({ company_id: null });

    await waitFor(() =>
      expect(meet()).toHaveAccessibleDescription(en["deal.meetNeedsCompany"]),
    );
    expect(meet()).toBeDisabled();
  });

  it("offers Meet with no refusal on a deal with a company", async () => {
    drawActions({ company_id: "01a02000-0000-7000-8000-000000000001" });

    await waitFor(() => expect(meet()).toBeEnabled());
    expect(meet()).not.toHaveAccessibleDescription(en["deal.meetNeedsCompany"]);
  });
});
