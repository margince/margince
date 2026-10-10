/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import { meFixture } from "../app/mefixture";
import type { ReviewTemplate } from "./outcomereview.queries";
import { ReviewTemplatesCard } from "./reviewtemplates";
import { jsonResponse, render } from "./settings.testkit";

// What a closed deal is asked, one group per template, so an administrator
// reads the questions under the template that asks them.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

function template(over: Partial<ReviewTemplate>): ReviewTemplate {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    key: "win_review",
    label: "Win review",
    outcome: "won",
    questions: [
      { key: "why", label: "Why did we win?", type: "text", required: true },
      {
        key: "else",
        label: "Who else were they considering?",
        type: "text",
        required: false,
      },
    ],
    version: 1,
    active: true,
    system: true,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  };
}

const TEMPLATES = [
  template({}),
  template({
    id: "22222222-2222-4222-8222-222222222222",
    key: "loss_review",
    label: "Loss review",
    outcome: "lost",
    questions: [
      {
        key: "why_lost",
        label: "What decided it against us?",
        type: "text",
        required: false,
      },
    ],
  }),
];

function served(allow: GrantSpec) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ roles: ["admin"], allow }));
      }
      if (url.includes("/activity-review-templates")) {
        return jsonResponse({ data: TEMPLATES });
      }
      return jsonResponse({ data: [], page: {} });
    }),
  );
}

const EDITOR: GrantSpec = { custom_field: ["read", "update"] };

describe("outcome review questions", () => {
  it("lists each template's questions in order under that template's own heading", async () => {
    served(EDITOR);
    render(<ReviewTemplatesCard />);

    const win = await screen.findByRole("list", { name: "Win review" });
    expect(
      within(win)
        .getAllByRole("listitem")
        .map(
          (item) => item.querySelector(".reviewtemplate-label")?.textContent,
        ),
    ).toEqual(["Why did we win?", "Who else were they considering?"]);
    const loss = screen.getByRole("list", { name: "Loss review" });
    expect(win.tagName).toBe("OL");
    expect(within(loss).getAllByRole("listitem")).toHaveLength(1);
  });

  it("marks only the questions that must be answered", async () => {
    served(EDITOR);
    render(<ReviewTemplatesCard />);

    const win = await screen.findByRole("list", { name: "Win review" });
    const [required, optional] = within(win).getAllByRole("listitem");
    expect(within(required).getByText("Required")).toBeTruthy();
    expect(within(optional).queryByText("Required")).toBeNull();
    expect(
      within(screen.getByRole("list", { name: "Loss review" })).queryByText(
        "Required",
      ),
    ).toBeNull();
  });

  it("puts each template's edit verb in its own head and opens that template", async () => {
    served(EDITOR);
    const user = userEvent.setup();
    render(<ReviewTemplatesCard />);

    const lossHead = await screen.findByRole("heading", {
      name: "Loss review",
    });
    const head = lossHead.closest<HTMLElement>(".panel-grouphead");
    if (!head) throw new Error("the template name is not a group head");
    await user.click(
      within(head).getByRole("button", { name: "Edit questions" }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("heading", { name: "Loss review" }),
    ).toBeTruthy();
  });

  it("offers no edit verb to a reader who may only read the questions", async () => {
    served({ custom_field: ["read"] });
    render(<ReviewTemplatesCard />);

    await screen.findByRole("list", { name: "Win review" });
    expect(screen.queryByRole("button", { name: "Edit questions" })).toBeNull();
  });
});
