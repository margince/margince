/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { project360 } from "../../testing/projects.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { ProjectFigures } from "./projectreadings";
import { deliveringProject360 } from "./projectstanding.fixtures";

// The figures under a project's How it stands. Each cell states one reading
// from the payload, and an empty or withheld one says so in words.

afterEach(cleanup);

function plate(view: ReturnType<typeof project360>): HTMLElement {
  render(
    <StoryProviders>
      <ProjectFigures view={view} />
    </StoryProviders>,
  );
  return screen.getByTestId("project-figures");
}

function cell(strip: HTMLElement, label: string): HTMLElement {
  const card = within(strip).getByText(label).closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error(`no figure named ${label}`);
  }
  return card;
}

describe("the project's figures", () => {
  it("reads phase time, open commitments and the files", () => {
    const strip = plate(deliveringProject360());
    expect(strip.childElementCount).toBe(5);
    expect(
      within(cell(strip, en["project.standing.phaseTime"])).getByText(
        "134 days",
      ),
    ).toBeTruthy();
    const commitments = cell(strip, en["record.commitments"]);
    expect(within(commitments).getByText("3 open")).toBeTruthy();
    expect(within(commitments).getByText("1 overdue")).toBeTruthy();
    const documents = cell(strip, en["project.documents.title"]);
    expect(within(documents).getByText("1")).toBeTruthy();
    expect(within(documents).getByText("Site report")).toBeTruthy();
  });

  it("says none is open and none is attached rather than a zero", () => {
    const strip = plate(
      project360({
        rollups: undefined,
      }),
    );
    expect(
      within(cell(strip, en["record.commitments"])).getByText(
        en["project.standing.noneOpen"],
      ),
    ).toBeTruthy();
    expect(
      within(cell(strip, en["project.documents.title"])).getByText(
        en["project.standing.noDocuments"],
      ),
    ).toBeTruthy();
  });

  it("says withheld on the commitments a reader's role cannot read", () => {
    const strip = plate(
      project360({ sections_omitted: ["commitments"], commitments: undefined }),
    );
    expect(
      within(cell(strip, en["record.commitments"])).getByText(
        en["state.withheld"],
      ),
    ).toBeTruthy();
  });

  it("says the files did not load when the read failed", () => {
    const strip = plate(project360({ documents: undefined }));
    expect(
      within(cell(strip, en["project.documents.title"])).getByText(
        en["project.standing.documentsUnavailable"],
      ),
    ).toBeTruthy();
  });
});

it("shows the server deal values and names an unpriced shortfall", () => {
  const strip = plate(
    project360({
      rollups: {
        open_deal_value: { amount_minor: 125_000, currency: "EUR" },
        won_deal_value: { amount_minor: null, currency: "EUR" },
        open_deals_unpriced: 1,
        won_deals_unpriced: 2,
        open_commitments: 0,
        last_activity_at: null,
        activity_count: 0,
      },
    }),
  );
  expect(
    within(cell(strip, en["project.openDealValue"])).getByText("€1,250.00"),
  ).toBeTruthy();
  expect(
    within(cell(strip, en["project.openDealValue"])).getByText(
      "1 deal not priced · excluded",
    ),
  ).toBeTruthy();
  expect(
    within(cell(strip, en["project.wonDealValue"])).getByText("—"),
  ).toBeTruthy();
  expect(
    within(cell(strip, en["project.wonDealValue"])).getByText(
      "2 deals not priced · excluded",
    ),
  ).toBeTruthy();
});
