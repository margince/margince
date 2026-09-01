/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { IntroRequestModal, type IntroTarget } from "./introrequest";

afterEach(cleanup);

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const TARGET: IntroTarget = {
  personId: "p-1",
  personName: "Philipp Königs",
  viaUserId: "u-1",
  viaName: "Sofia Meier",
};

function stub(draft: unknown, status = 200) {
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request = input as Request;
    const url = typeof input === "string" ? input : request.url;
    calls.push({ url, body: request.text ? await request.text() : "" });
    return new Response(JSON.stringify(draft), {
      status,
      headers: {
        "content-type":
          status >= 400 ? "application/problem+json" : "application/json",
      },
    });
  });
  return calls;
}

const WRITTEN = {
  subject: "Could you introduce me to Philipp Königs?",
  body: "Hi Sofia,\n\nYou and Philipp Königs have been in touch. Would you introduce us?",
  generated_by: "model",
  ai_generated: true,
  reasoning: [
    {
      kind: "relationship",
      label: "Sofia Meier knows Philipp Königs (strong)",
    },
  ],
};

test("names who is being asked, and about whom, before anything is written", async () => {
  stub(WRITTEN);
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  expect(
    await screen.findByText(
      /Asking Sofia Meier to introduce you to Philipp Königs/,
    ),
  ).toBeTruthy();
});

// The draft carries the deal when one is selected, because an introduction for
// a specific transaction says so and one for the account does not.
test("sends the deal when there is one, and both required ids", async () => {
  const calls = stub(WRITTEN);
  const user = userEvent.setup();
  render(
    <IntroRequestModal
      orgId="o-1"
      target={TARGET}
      dealId="d-1"
      onClose={() => {}}
    />,
  );
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  await screen.findByDisplayValue(/Could you introduce me to Philipp Königs/);
  const sent = calls.find((call) => call.url.includes("intro-request-draft"));
  expect(sent).toBeTruthy();
  expect(JSON.parse(sent?.body ?? "{}")).toEqual({
    person_id: "p-1",
    via_user_id: "u-1",
    deal_id: "d-1",
  });
});

// A reader who edits owns the words. The tag has to follow, or the product
// keeps claiming authorship of a sentence a person wrote.
test("the message becomes the reader's once they edit it", async () => {
  stub(WRITTEN);
  const user = userEvent.setup();
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );

  const body = await screen.findByLabelText("Message");
  expect(screen.queryByText(/typed by you/i)).toBeNull();
  // A REWRITE, not a typo fix. One keystroke is not authorship, and claiming
  // it would credit the reader with a message the model wrote.
  await user.type(body, " Thanks — happy to explain what it is about first.");
  expect(await screen.findByText(/typed by you/i)).toBeTruthy();
});

// One keystroke is not authorship. Flipping the mark on the first character
// credits the reader with a message the model wrote — the same misattribution
// the mark exists to prevent, in the other direction.
test("a typo fix does not claim the message as the reader's", async () => {
  stub(WRITTEN);
  const user = userEvent.setup();
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  await user.type(await screen.findByLabelText("Message"), "!");
  expect(screen.queryByText(/typed by you/i)).toBeNull();
});

// A browser that refuses clipboard access must SAY so. A button that silently
// does nothing leaves a reader pressing it and wondering why their paste is
// empty.
test("says so when the browser will not let the page copy", async () => {
  stub(WRITTEN);
  const user = userEvent.setup();
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  // AFTER setup, which installs a clipboard of its own — taken away here to
  // model the browser that never offered one.
  const original = navigator.clipboard;
  Object.defineProperty(navigator, "clipboard", {
    value: undefined,
    configurable: true,
  });
  await user.click(await screen.findByRole("button", { name: /^Copy$/ }));
  expect(await screen.findByText(/would not let the page copy/i)).toBeTruthy();
  Object.defineProperty(navigator, "clipboard", {
    value: original,
    configurable: true,
  });
});

// With no model configured the endpoint answers from a template. Saying so is
// the difference between a reader trusting the phrasing and wondering why it
// reads flat.
test("says when the message came from a template rather than a model", async () => {
  stub({ ...WRITTEN, generated_by: "deterministic", ai_generated: false });
  const user = userEvent.setup();
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  expect(await screen.findByText(/Written from a template/)).toBeTruthy();
});

// A refusal has to reach the reader. The endpoint answers 404 for a colleague
// with no recorded route, and a dialog that swallowed it would look broken.
test("shows the refusal rather than an empty form", async () => {
  stub({ code: "not_found", detail: "no such route" }, 404);
  const user = userEvent.setup();
  render(<IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />);
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  // The SERVER'S OWN WORDS. Asserting only that some text appeared passed
  // against a version that threw the raw body, where every refusal — 403, 404
  // and 422 alike — collapsed into "the request failed, no cause reported".
  expect(await screen.findByText(/no such route/)).toBeTruthy();
  expect(screen.queryByLabelText("Message")).toBeNull();
});

test("renders in German under a German locale", async () => {
  stub(WRITTEN);
  render(
    <IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />,
    "de",
  );
  expect(await screen.findByText("Um eine Vorstellung bitten")).toBeTruthy();
});

// A DRAFT MUST NOT FOLLOW THE READER TO ANOTHER CONTACT.
//
// The dialog holds a draft and the reader's edits, and React reuses a
// component across prop changes. Drafting for one person and then opening
// another showed the FIRST person's message under the second one's name —
// which a reader would send. The screen keys the dialog on who is being asked
// about; this proves the dialog itself starts clean when it does.
test("a fresh target starts with no draft", async () => {
  stub(WRITTEN);
  const user = userEvent.setup();
  const first = render(
    <IntroRequestModal orgId="o-1" target={TARGET} onClose={() => {}} />,
  );
  await user.click(
    await screen.findByRole("button", { name: /Write the message/i }),
  );
  await screen.findByLabelText("Message");
  first.unmount();

  render(
    <IntroRequestModal
      orgId="o-1"
      target={{ ...TARGET, personId: "p-2", personName: "Jan Roth" }}
      onClose={() => {}}
    />,
  );
  // No message yet: the verb is offered, not somebody else's draft.
  expect(screen.queryByLabelText("Message")).toBeNull();
  expect(
    await screen.findByRole("button", { name: /Write the message/i }),
  ).toBeTruthy();
});
