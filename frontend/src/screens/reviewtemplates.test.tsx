/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RbacAction } from "../app/capability";
import type { ReviewTemplate } from "./outcomereview.queries";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import { ReviewTemplatesCard } from "./reviewtemplates";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const NATIVE_FETCH = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = NATIVE_FETCH;
  vi.unstubAllGlobals();
});

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
      { key: "why", label: "Why did we lose?", type: "text", required: true },
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

function serveCard(allow: readonly RbacAction[], templates: ReviewTemplate[]) {
  installFetchStub({
    "GET /me": meRoute({ custom_field: allow }),
    "GET /activity-review-templates": () => jsonResponse({ data: templates }),
  });
  render(
    <StoryProviders>
      <ReviewTemplatesCard />
    </StoryProviders>,
  );
}

describe("the outcome reviews page", () => {
  it("gives each review its own panel with numbered questions and what each needs", async () => {
    serveCard(["read"], [template()]);
    const panel = await screen.findByRole("region", { name: "Loss review" });
    expect(within(panel).getByText("Lost")).toBeTruthy();
    expect(within(panel).getByText("2 questions, 1 required")).toBeTruthy();
    const rows = within(panel).getAllByRole("row").slice(1);
    expect(within(rows[0]).getByText("Why did we lose?")).toBeTruthy();
    expect(
      within(rows[0]).getByText(
        "Filled with the close reason, when the rep gave one",
      ),
    ).toBeTruthy();
    // The answer and need columns; the question cell repeats both for a
    // narrow panel, where the stylesheet folds these columns away.
    const [first, second] = rows.map((row) => within(row).getAllByRole("cell"));
    expect(first[3].textContent).toBe("Required");
    expect(second[2].textContent).toBe("Pick from a list");
    expect(second[3].textContent).toBe("Optional");
  });

  it("offers Edit questions only to a reader who may update custom fields", async () => {
    serveCard(["read"], [template()]);
    await screen.findByRole("region", { name: "Loss review" });
    expect(screen.queryByRole("button", { name: "Edit questions" })).toBeNull();
    cleanup();
    serveCard(["read", "update"], [template()]);
    const panel = await screen.findByRole("region", { name: "Loss review" });
    expect(
      await within(panel).findByRole("button", { name: "Edit questions" }),
    ).toBeTruthy();
  });

  it("keeps a retired review listed under a Retired badge", async () => {
    serveCard(["read"], [template({ active: false })]);
    const panel = await screen.findByRole("region", { name: "Loss review" });
    expect(within(panel).getByText("Retired")).toBeTruthy();
  });

  it("links the outcome note to the pipelines and stage automation pages", async () => {
    serveCard(["read"], [template()]);
    await screen.findByRole("region", { name: "Loss review" });
    const pipelines = screen.getByRole("link", { name: "Pipelines" });
    const automation = screen.getByRole("link", { name: "Stage automation" });
    expect(pipelines.getAttribute("href")).toContain("pipelines");
    expect(automation.getAttribute("href")).toContain("stageautomation");
  });

  it("says no review is offered when nothing is set up", async () => {
    serveCard(["read"], []);
    expect(
      await screen.findByText(
        "No review questions are set up. Reps are not offered a review when they close a deal.",
      ),
    ).toBeTruthy();
  });
});

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
    await user.clear(screen.getAllByRole("textbox", { name: /Question/ })[0]);
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
    openEditor(template({ questions: [template().questions[0]] }));
    expect(
      screen
        .getByRole("button", { name: "Remove question 1" })
        .hasAttribute("disabled"),
    ).toBe(true);
  });
});
