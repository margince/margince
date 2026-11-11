/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { WordingCorrectionModal } from "./wordingcorrectionmodal";

const approved: components["schemas"]["WordingCorrectionContext"] = {
  instruction_id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  version: 7,
  subject: "Requested information",
  body: "Thank yuo!",
  html_body: "<p>Thank yuo!</p>",
  wording_digest: "a".repeat(64),
  reason_code: "contractual_necessity",
  explanation: "The contract requires this notice.",
  warning_version: "override-v1",
};

function draw(onSaved = vi.fn(), context = approved) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { mutations: { retry: false } } })
      }
    >
      <WordingCorrectionModal
        reviewId="review-1"
        context={context}
        onClose={vi.fn()}
        onSaved={onSaved}
      />
    </QueryClientProvider>,
  );
  return onSaved;
}

afterEach(() => vi.unstubAllGlobals());

it("requires an edit and an attestation about the final wording", async () => {
  draw();
  const user = userEvent.setup();
  const save = () => screen.getByRole("button", { name: "Save correction" });
  const confirmation = screen.getByRole("checkbox", {
    name: /only spelling or punctuation changed/i,
  });
  expect(confirmation).not.toBeChecked();
  await user.click(confirmation);
  expect(save()).toBeDisabled();
  await user.type(screen.getByRole("textbox", { name: "Subject" }), ".");
  expect(confirmation).not.toBeChecked();
  expect(save()).toBeDisabled();
  await user.click(confirmation);
  expect(save()).not.toBeDisabled();
  await user.type(screen.getByRole("textbox", { name: "Subject" }), "!");
  expect(confirmation).not.toBeChecked();
  expect(save()).toBeDisabled();
});

it("sends both corrected alternatives with the observed approval fingerprint and version", async () => {
  let submitted: unknown;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      expect(request.url).toContain(
        "/communication-reviews/review-1/correct-wording",
      );
      submitted = await request.json();
      return new Response(JSON.stringify({ version: 8 }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  const saved = draw();
  const user = userEvent.setup();
  const editor = screen.getByRole("textbox", { name: "Body" });
  editor.innerHTML = "<p>Thank you!</p>";
  fireEvent.input(editor);
  await user.click(
    screen.getByRole("checkbox", {
      name: /only spelling or punctuation changed/i,
    }),
  );
  await user.click(screen.getByRole("button", { name: "Save correction" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(submitted).toEqual({
    version: 7,
    expected_wording: approved.wording_digest,
    subject: approved.subject,
    body: "Thank you!",
    html_body: "<p>Thank you!</p>",
    meaning_unchanged: true,
  });
});

it("keeps a plain-only message plain after a wording correction", async () => {
  let submitted: unknown;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      submitted = await request.json();
      return new Response(JSON.stringify({ version: 8 }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  const saved = draw(vi.fn(), { ...approved, html_body: "" });
  const user = userEvent.setup();
  const editor = screen.getByRole("textbox", { name: "Body" });
  await user.clear(editor);
  await user.type(editor, "Thank you!");
  await user.click(
    screen.getByRole("checkbox", {
      name: /only spelling or punctuation changed/i,
    }),
  );
  await user.click(screen.getByRole("button", { name: "Save correction" }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(submitted).toMatchObject({ body: "Thank you!", html_body: "" });
});
