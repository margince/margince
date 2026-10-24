/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import { deliveringProject360 } from "./projectstanding.fixtures";
import { ProjectStanding } from "./projectstandingcard";

afterEach(cleanup);

describe("the project's phase ladder", () => {
  it("writes each rung's days and a dash for a rung never reached", () => {
    render(
      <StoryProviders>
        <ProjectStanding
          view={deliveringProject360()}
          onMove={() => undefined}
          onOpenDocuments={() => undefined}
        />
      </StoryProviders>,
    );
    const ladder = within(screen.getByRole("group", { name: "Phase" }));
    expect(ladder.getByText("31 days")).toBeTruthy();
    expect(ladder.getByText("134 days · now")).toBeTruthy();
    // Pursuing was skipped; Closed is a way out and carries no line.
    expect(ladder.getAllByText("—")).toHaveLength(1);
  });
});
