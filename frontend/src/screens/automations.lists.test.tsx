// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { RulePausedReason } from "./automations.lists";
import { paramFields } from "./automations.params";
import { StoryProviders } from "./story-utils";

type Automation = components["schemas"]["Automation"];

const rule: Automation = {
  id: "01a0f000-0000-7000-8000-000000000031",
  key: "list_membership_notify",
  name: "Tell me about new buyers",
  status: "paused",
  params: { list_id: "01a0f000-0000-7000-8000-000000000001" },
  created_at: "2026-09-20T08:00:00Z",
};

afterEach(cleanup);

describe("a rule that watches a list", () => {
  it("draws a list picker for a property that names a list", () => {
    expect(
      paramFields({
        type: "object",
        properties: {
          list_id: { type: "string", format: "live_list" },
          shortlist_id: { type: "string", format: "shortlist" },
          direction: {
            type: "string",
            enum: ["entered", "left", "either"],
            default: "entered",
          },
        },
      }).map((field) => [field.key, field.kind]),
    ).toEqual([
      ["list_id", "live_list"],
      ["shortlist_id", "shortlist"],
      ["direction", "enum"],
    ]);
  });

  it("says why it paused itself, and nothing when paused by hand", () => {
    const { rerender } = render(
      <StoryProviders>
        <RulePausedReason automation={{ ...rule, paused_reason: "burst" }} />
      </StoryProviders>,
    );
    expect(screen.getByText(en["auto.pausedReason.burst"])).toBeInTheDocument();
    rerender(
      <StoryProviders>
        <RulePausedReason automation={rule} />
      </StoryProviders>,
    );
    expect(
      screen.queryByText(en["auto.pausedReason.burst"]),
    ).not.toBeInTheDocument();
  });
});
