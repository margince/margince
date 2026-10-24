/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { ReviewTemplate } from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";

const NATIVE_FETCH = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = NATIVE_FETCH;
  vi.unstubAllGlobals();
});

const WHY = {
  key: "why",
  label: "Why did we lose?",
  type: "text",
  required: true,
} as const;

function template(over: Partial<ReviewTemplate> = {}): ReviewTemplate {
  return {
    id: "loss",
    key: "loss_review",
    label: "Loss review",
    outcome: "lost",
    version: 4,
    active: true,
    system: true,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
    questions: [
      WHY,
      {
        key: "who",
        label: "Who did they choose instead?",
        type: "multiselect",
        required: false,
        options: ["A competitor"],
      },
    ],
    ...over,
  };
}

function openEditor(current: ReviewTemplate, onPatch = vi.fn()) {
  installFetchStub({
    "GET /me": meRoute({ custom_field: ["read", "update"] }),
    [`PATCH /activity-review-templates/${current.id}`]: (body) => {
      onPatch(body);
      return jsonResponse(current);
    },
  });
  const close = vi.fn();
  render(
    <StoryProviders>
      <ReviewTemplateEditor template={current} onClose={close} />
    </StoryProviders>,
  );
  return { close, user: userEvent.setup() };
}

describe("editing a review's questions", () => {
  it("saves the reordered questions with trimmed choices and the version it read", async () => {
    const onPatch = vi.fn();
    const { close, user } = openEditor(
      template({
        questions: [
          { key: "why", label: "Why?", type: "text", required: true },
          {
            key: "who",
            label: "Who?",
            type: "multiselect",
            required: false,
            options: ["  A competitor ", "", "In-house"],
          },
        ],
      }),
      onPatch,
    );
    await user.click(
      screen.getByRole("button", { name: "Move question 2 up" }),
    );
    expect(screen.getByText("Unsaved changes")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Save questions" }));
    await vi.waitFor(() => expect(close).toHaveBeenCalled());
    expect(onPatch).toHaveBeenCalledWith({
      version: 4,
      questions: [
        {
          key: "who",
          label: "Who?",
          type: "multiselect",
          required: false,
          options: ["A competitor", "In-house"],
        },
        { key: "why", label: "Why?", type: "text", required: true },
      ],
    });
  });

  it("holds Save while a question has no label", async () => {
    const { user } = openEditor(template());
    const save = screen.getByRole("button", { name: "Save questions" });
    expect(save.hasAttribute("disabled")).toBe(false);
    const [first] = screen.getAllByRole("textbox", { name: /Question/ });
    if (!first) throw new Error("the drawer shows no question box");
    await user.clear(first);
    expect(save.hasAttribute("disabled")).toBe(true);
  });

  it("holds Save while a pick-from-a-list question has no choice", async () => {
    const { user } = openEditor(template());
    await user.clear(
      screen.getByRole("textbox", { name: /Choices, one per line/ }),
    );
    expect(
      screen
        .getByRole("button", { name: "Save questions" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("will not remove the last remaining question", () => {
    openEditor(template({ questions: [WHY] }));
    expect(
      screen
        .getByRole("button", { name: "Remove question 1" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });

  it("stages Required as a box that rides the drawer's Save", async () => {
    const onPatch = vi.fn();
    const { user } = openEditor(template(), onPatch);
    const [first] = screen.getAllByRole("checkbox", { name: "Required" });
    if (!first) throw new Error("the drawer shows no Required box");
    expect(screen.queryByRole("switch")).toBeNull();
    await user.click(first);
    expect(onPatch).not.toHaveBeenCalled();
    expect(screen.getByText("Unsaved changes")).toBeTruthy();
  });
});
