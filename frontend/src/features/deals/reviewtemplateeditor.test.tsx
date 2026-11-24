/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { type Locale, translate, useLocale } from "../../i18n";
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

function openEditor(
  current: ReviewTemplate,
  onPatch = vi.fn(),
  locale: Locale = "en",
) {
  installFetchStub({
    "GET /me": meRoute({ custom_field: ["read", "update"] }),
    [`PATCH /activity-review-templates/${current.id}`]: (body) => {
      onPatch(body);
      return jsonResponse(current);
    },
  });
  const close = vi.fn();
  render(
    <StoryProviders locale={locale}>
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

  it("shows a built-in question in the reader's language and saves the stored words unless reworded", async () => {
    const onPatch = vi.fn();
    const { close, user } = openEditor(
      template({
        questions: [
          {
            ...WHY,
            label_key: "reviewTemplates.builtin.question.why_we_lost",
          },
          {
            key: "lost_to",
            label: "Who did they choose instead?",
            label_key: "reviewTemplates.builtin.question.lost_to",
            type: "text",
            required: false,
          },
        ],
      }),
      onPatch,
      "vi",
    );
    const [why, lostTo] = screen.getAllByRole("textbox", {
      name: new RegExp(translate("vi", "reviewTemplates.question")),
    });
    if (!why || !lostTo) throw new Error("the drawer shows no question box");
    expect(why).toHaveProperty("value", "Vì sao thua deal này?");
    await user.clear(lostTo);
    await user.type(lostTo, "Bên nào thắng?");
    await user.click(
      screen.getByRole("button", {
        name: translate("vi", "reviewTemplates.saveQuestions"),
      }),
    );
    await vi.waitFor(() => expect(close).toHaveBeenCalled());
    const [body] = onPatch.mock.calls[0] ?? [];
    expect(body.questions.map((q: { label: string }) => q.label)).toEqual([
      "Why did we lose?",
      "Bên nào thắng?",
    ]);
  });

  // The words were shown in the language the drawer opened in. A switch with
  // the drawer open is no edit, so the built-in question keeps its key.
  it("keeps a built-in question's stored words when the language changes with the drawer open", async () => {
    const onPatch = vi.fn();
    let switchTo: (locale: Locale) => void = () => {};
    function LanguageSwitch() {
      switchTo = useLocale().setLocale;
      return null;
    }
    installFetchStub({
      "GET /me": meRoute({ custom_field: ["read", "update"] }),
      "PATCH /activity-review-templates/loss": (body) => {
        onPatch(body);
        return jsonResponse(template());
      },
    });
    render(
      <StoryProviders locale="en">
        <LanguageSwitch />
        <ReviewTemplateEditor
          template={template({
            questions: [
              {
                ...WHY,
                label_key: "reviewTemplates.builtin.question.why_we_lost",
              },
            ],
          })}
          onClose={vi.fn()}
        />
      </StoryProviders>,
    );
    act(() => switchTo("de"));
    expect(
      screen.queryByText(translate("de", "reviewTemplates.unsaved")),
    ).toBeNull();
    await userEvent.setup().click(
      screen.getByRole("button", {
        name: translate("de", "reviewTemplates.saveQuestions"),
      }),
    );
    await vi.waitFor(() => expect(onPatch).toHaveBeenCalled());
    const [body] = onPatch.mock.calls[0] ?? [];
    expect(body.questions[0].label).toBe("Why did we lose?");
  });

  it("says a question with no label is required on Save, and sends nothing", async () => {
    const onPatch = vi.fn();
    const { user } = openEditor(template(), onPatch);
    const [first] = screen.getAllByRole("textbox", { name: /Question/ });
    if (!first) throw new Error("the drawer shows no question box");
    await user.clear(first);
    await user.click(screen.getByRole("button", { name: "Save questions" }));
    expect(screen.getByText("Question is required.")).toBeTruthy();
    expect(document.activeElement).toBe(first);
    expect(onPatch).not.toHaveBeenCalled();
  });

  it("says a pick-from-a-list question needs a choice on Save", async () => {
    const onPatch = vi.fn();
    const { user } = openEditor(template(), onPatch);
    const choices = screen.getByRole("textbox", {
      name: /Choices, one per line/,
    });
    await user.clear(choices);
    await user.click(screen.getByRole("button", { name: "Save questions" }));
    expect(screen.getByText("This field is required.")).toBeTruthy();
    expect(document.activeElement).toBe(choices);
    expect(onPatch).not.toHaveBeenCalled();
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

describe("adding a review template", () => {
  function openNew(onPost = vi.fn()) {
    installFetchStub({
      "GET /me": meRoute({ custom_field: ["read", "create", "update"] }),
      "POST /activity-review-templates": (body) => {
        onPost(body);
        return jsonResponse(template({ id: "new", system: false }), 201);
      },
    });
    const close = vi.fn();
    render(
      <StoryProviders>
        <ReviewTemplateEditor onClose={close} />
      </StoryProviders>,
    );
    return { close, user: userEvent.setup() };
  }

  it("posts the name, the outcome it is asked for and its questions", async () => {
    const onPost = vi.fn();
    const { close, user } = openNew(onPost);
    expect(
      screen.getByRole("heading", { name: "New review template" }),
    ).toBeTruthy();
    const add = screen.getByRole("button", { name: "Add template" });
    await user.click(add);
    expect(screen.getByText("Name is required.")).toBeTruthy();
    expect(onPost).not.toHaveBeenCalled();

    await user.type(
      screen.getByRole("textbox", { name: /Name/ }),
      "  Enterprise loss  ",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Asked for" }),
      "Lost deals",
    );
    await user.type(
      screen.getByRole("textbox", { name: /Question/ }),
      "What did we miss?",
    );
    await user.click(add);
    await vi.waitFor(() => expect(close).toHaveBeenCalled());
    expect(onPost).toHaveBeenCalledWith({
      label: "Enterprise loss",
      outcome: "lost",
      questions: [
        expect.objectContaining({
          label: "What did we miss?",
          type: "text",
          required: false,
        }),
      ],
    });
  });
});
