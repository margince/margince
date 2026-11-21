// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SearchedRecordValue } from "./filtervalue.records";

afterEach(cleanup);

const ID = "01a1e510-c819-70a4-80a4-700299ba3784";

describe("a company clause restored from a saved view", () => {
  it("shows the company's name and names its remove button, never the id", async () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      ...recordNamesRoutes({ company: { [ID]: "Demo GmbH" } }),
    });
    render(
      <StoryProviders>
        <SearchedRecordValue many value={[ID]} onChange={vi.fn()} />
      </StoryProviders>,
    );
    expect(await screen.findByText("Demo GmbH")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Remove Demo GmbH" }),
    ).toBeInTheDocument();
    expect(screen.queryByText(ID)).not.toBeInTheDocument();
  });
});
