/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import {
  aiJudgedProject360,
  deliveringProject360,
} from "./projectstanding.fixtures";
import { ProjectStanding } from "./projectstandingcard";

afterEach(cleanup);

function standing(view: ReturnType<typeof deliveringProject360>) {
  render(
    <StoryProviders>
      <ProjectStanding
        view={view}
        onMove={() => undefined}
        onOpenDocuments={() => undefined}
      />
    </StoryProviders>,
  );
}

describe("a health reading the health job wrote", () => {
  it("carries the agent tag, its reason and an undo", () => {
    standing(aiJudgedProject360());
    expect(screen.getByText("Automated by Project health check")).toBeTruthy();
    expect(
      screen.getByText(
        "Nothing has been filed on this project for at least 30 days.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Undo the AI reading" }),
    ).toBeTruthy();
  });

  it("is not claimed for a user's reading", () => {
    standing(deliveringProject360());
    expect(screen.queryByText("Automated by Project health check")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Undo the AI reading" }),
    ).toBeNull();
  });
});

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
