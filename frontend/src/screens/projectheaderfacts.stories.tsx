// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProjectIdentityFacts } from "./projectheaderfacts";
import { COMPANY, project, project360 } from "./projects.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The facts beside a project's name: which account the work is for, when it is
// due and how far off that is, and whose project it is — the named cells that
// replaced a running, dot-joined identity line.
//
// Both states of the target-end cell are drawn below, because a cell that
// vanishes when nobody has set a date reads as a project with no date
// outstanding rather than one that needs a date.

const meta: Meta = {
  title: "Records/Project 360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The roster and the company name each resolve through their own read, the same
// ones every EntityRef in the app uses. Without them these frames would
// document the unresolved fallbacks as the normal state.
function installReferenceReads(): void {
  installFetchStub({
    "GET /users": () =>
      jsonResponse({
        data: [
          {
            id: "u-me",
            display_name: "Sofia Meier",
            email: "sofia@example.com",
          },
        ],
        page: { next_cursor: null },
      }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: COMPANY.id, display_name: COMPANY.display_name }],
        page: { next_cursor: null },
      }),
  });
}

export const Facts: Story = {
  render: () => {
    installReferenceReads();
    return (
      <StoryProviders>
        <ProjectIdentityFacts
          view={project360({
            project: project({ target_end_date: "2026-11-30" }),
          })}
          locale="en"
        />
      </StoryProviders>
    );
  },
};

/**
 * No target end set, and the account withheld from this reader.
 *
 * Both are stated rather than dropped. A missing date cell would answer "when
 * is this due" with silence, and a project drawn with no account reads as a
 * project that HAS none — a different claim from "you may not see it".
 */
export const FactsUnsetAndWithheld: Story = {
  render: () => {
    installReferenceReads();
    return (
      <StoryProviders>
        <ProjectIdentityFacts
          view={project360({
            sections_omitted: ["company"],
            company: undefined,
            project: project({ company_id: null, target_end_date: null }),
          })}
          locale="en"
        />
      </StoryProviders>
    );
  },
};
