/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import type { ReviewTemplate } from "../outcomereview.queries";
import { OutcomeReviewModal } from "./outcomereviewmodal";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("submits each choice intact and retains the opening template and closing", async () => {
  const submitted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      submitted.push(await req.json());
      return new Response(JSON.stringify({ id: "review" }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const template: ReviewTemplate = {
    id: "template",
    key: "win_review",
    label: "Win review",
    outcome: "won",
    version: 2,
    active: true,
    system: true,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
    questions: [
      {
        key: "reasons",
        label: "Reasons",
        type: "multiselect",
        required: true,
        options: ["Fit, scope", "Trust"],
      },
    ],
  };
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  const close = vi.fn();
  const view = (current: ReviewTemplate, closing: string) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider>
        <OutcomeReviewModal
          open
          onClose={close}
          dealId="deal"
          closingOccurrenceId={closing}
          template={current}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const rendered = render(view(template, "first-close"));
  const user = userEvent.setup();
  await user.click(screen.getByLabelText("Fit, scope"));
  await user.click(screen.getByLabelText("Trust"));
  rendered.rerender(
    view({ ...template, version: 3, questions: [] }, "later-close"),
  );
  await user.click(screen.getByRole("button", { name: "Save review" }));
  await vi.waitFor(() => expect(submitted).toHaveLength(1));
  expect(submitted[0]).toMatchObject({
    closing_occurrence_id: "first-close",
    template_version: 2,
    answers: {},
    choice_answers: { reasons: ["Fit, scope", "Trust"] },
  });
});
