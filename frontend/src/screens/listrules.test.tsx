// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { ListRuleUses, ruleUsesOf } from "./listrules";
import { liveList } from "./lists.fixtures";
import { StoryProviders } from "./story-utils";

afterEach(cleanup);

const withRules = {
  ...liveList,
  dependencies: [
    {
      kind: "export" as const,
      occurred_at: "2026-09-01T00:00:00Z",
      blocking: false,
    },
    {
      kind: "automation" as const,
      occurred_at: "2026-09-02T00:00:00Z",
      blocking: false,
      role: "writes" as const,
      automation_id: "01a0f000-0000-7000-8000-000000000041",
      automation_name: "Collect new buyers",
    },
    {
      kind: "automation" as const,
      occurred_at: "2026-09-03T00:00:00Z",
      blocking: false,
      role: "watches" as const,
    },
  ],
};

describe("the rules that use a list", () => {
  it("names each rule by what it does, and counts one the reader cannot open", () => {
    const rules = ruleUsesOf(withRules);
    expect(rules).toHaveLength(2);
    render(
      <StoryProviders>
        <ListRuleUses rules={rules} lead={en["lists.rules.settingsLead"]} />
      </StoryProviders>,
    );
    expect(
      screen.getByText(en["lists.rules.settingsLead"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Collect new buyers adds records to this list"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(`${en["lists.rules.hidden"]} watches this list`),
    ).toBeInTheDocument();
  });

  it("says nothing about a list no rule uses", () => {
    const { container } = render(
      <StoryProviders>
        <ListRuleUses
          rules={ruleUsesOf(liveList)}
          lead={en["lists.rules.settingsLead"]}
        />
      </StoryProviders>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
