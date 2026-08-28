// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../i18n/en";
import {
  type CompanyFieldName,
  type CompanyForm,
  EMPTY_DRAFT,
} from "./onboarding";
import { ManualCompanyInterview } from "./onboarding-manual-interview";
import { StoryProviders } from "./story-utils";

// The manual path through the company interview: one question at a time, its
// chapter and its position above it, and a hint under the prompt. It is the
// route somebody takes when the read found nothing — so every state here is a
// state of a person typing, not of a server answering.
//
// The position pair is what the stories move. `questionIndex` is the
// component's OWN state and starts at zero, so there is no prop that opens the
// interview in the middle: a story that wants question three has to answer its
// way there, which is also the only honest way to show that advancing works.

// Three questions refuse an empty answer — `display_name`, `offer_summary`
// and `icp` — and every one of them sits early in the run. So a story that
// walks forward has to have answered at least those, or the screen refuses the
// step by doing nothing and the story stops on a question its own name does not
// claim. The optional fields are left empty on purpose: an interview whose every
// answer is filled in cannot show the state a skippable question is in.
const ANSWERS: Partial<Record<CompanyFieldName, string>> = {
  legal_name: "Brandt Automotive GmbH",
  registered_address: "Werkstraße 14, 70565 Stuttgart",
  register_vat: "DE 812 456 991",
  display_name: "Brandt Automotive",
  offer_summary:
    "Retrofit lines for assembly plants, sold as a programme rather than a machine.",
  icp: "Tier-one suppliers running two or more plants on ageing lines.",
  industry: "Automotive tier-one supply",
};

// Built from the screen's own empty form rather than restated here: a story
// that spells the whole shape stops compiling every time the company gains a
// field, and says nothing about the story either way.
function form(overrides: Partial<CompanyForm> = {}): CompanyForm {
  return { ...EMPTY_DRAFT.values, ...overrides };
}

const meta: Meta<typeof ManualCompanyInterview> = {
  title: "Onboarding/Company interview/Manual",
  component: ManualCompanyInterview,
};
export default meta;
type Story = StoryObj<typeof ManualCompanyInterview>;

function interview(values: CompanyForm) {
  return () => (
    <StoryProviders>
      <ManualCompanyInterview
        values={values}
        setField={() => {}}
        onPersist={() => {}}
        onBackToChoice={() => {}}
        onComplete={() => {}}
      />
    </StoryProviders>
  );
}

type Walker = ReturnType<typeof userEvent.setup>;

// The one button that moves the interview on — and it is not one word. A
// question that is optional and still empty offers "Add later" where an
// answered or a required one offers "Next question", and skipping an optional
// question IS moving forward. Named from the catalog rather than restated: the
// verbs have been reworded before, and a walk that failed for the wording would
// be reporting on the copy rather than on the step.
function forwardVerb(canvasElement: HTMLElement): HTMLElement | null {
  const canvas = within(canvasElement);
  return (
    canvas.queryByRole("button", { name: en["ob.manualNext"] }) ??
    canvas.queryByRole("button", { name: en["ob.manualLater"] })
  );
}

function prompt(canvasElement: HTMLElement): string {
  const canvas = within(canvasElement);
  return canvas.getByRole("heading", { level: 1 }).textContent ?? "";
}

// One question forward, or a failure naming where the walk stuck.
//
// The screen refuses to leave a required question nobody has answered, and it
// refuses by doing nothing at all. A walk that took that for a step would go on
// counting and describe the wrong question; one that waited for it to pass
// would spin until the story timed out with nothing to read. So a step that
// leaves the same prompt on screen is reported as what it is.
async function step(canvasElement: HTMLElement, user: Walker): Promise<void> {
  const before = prompt(canvasElement);
  const verb = forwardVerb(canvasElement);
  if (!verb) {
    throw new Error(`the interview offers no way on from "${before}"`);
  }
  await user.click(verb);
  await waitFor(() => {
    expect(prompt(canvasElement)).not.toBe(before);
  });
}

// Advancing needs the current question ANSWERED when it is a required one, so
// the stories that walk forward start from a filled form.
async function advance(canvasElement: HTMLElement, times: number) {
  const user = userEvent.setup();
  for (let taken = 0; taken < times; taken += 1) {
    await step(canvasElement, user);
  }
}

// To the END, rather than by a count of steps. A number here is a second
// statement of how many questions there are, and it goes wrong the day one is
// added — which is how this story first failed, looking for a verb that had
// already become "Review my answers". The condition cannot: the run is over
// exactly when neither forward verb is on screen and only the review is left.
async function advanceToLast(canvasElement: HTMLElement) {
  const user = userEvent.setup();
  while (forwardVerb(canvasElement)) {
    await step(canvasElement, user);
  }
}

// To the first question the interview will take an empty answer for, by the
// SIGN of one rather than by a count: the verb reading "Add later" is exactly
// what makes a question skippable here, so asking for that verb is asking for
// the state the story is named after. A step count would have to be re-derived
// every time a question moves, and the walk would land quietly on a neighbour.
async function advanceToOptional(canvasElement: HTMLElement) {
  const user = userEvent.setup();
  const canvas = within(canvasElement);
  while (!canvas.queryByRole("button", { name: en["ob.manualLater"] })) {
    await step(canvasElement, user);
  }
}

/** The first question, on an empty form: the chapter, the position, and a
 *  prompt nobody has answered yet. */
export const FirstQuestion: Story = { render: interview(form()) };

/** Part-way through, so the position pair reads as a place rather than a
 *  count. Neither half is ever grouped — "Frage 1.204 von 1.018" would read as
 *  a quantity of questions. */
export const MidInterview: Story = {
  render: interview(form(ANSWERS)),
  play: async ({ canvasElement }) => advance(canvasElement, 2),
};

/** The last question: the verb stops saying "next" and offers the review
 *  instead, which is the only signal that the interview is about to end. */
export const LastQuestion: Story = {
  render: interview(form(ANSWERS)),
  play: async ({ canvasElement }) => advanceToLast(canvasElement),
};

/** The first question the interview will take an empty answer for. The line
 *  under the field says which kind it is, because "Add later" and a refused
 *  Next look the same until it does. */
export const OptionalQuestion: Story = {
  render: interview(form(ANSWERS)),
  play: async ({ canvasElement }) => advanceToOptional(canvasElement),
};

/** The German interview: the same step, the longer prompts, and the position
 *  pair in the reader's own notation. */
export const German: Story = {
  render: () => (
    <StoryProviders locale="de">
      <ManualCompanyInterview
        values={form(ANSWERS)}
        setField={() => {}}
        onPersist={() => {}}
        onBackToChoice={() => {}}
        onComplete={() => {}}
      />
    </StoryProviders>
  ),
};

/** At 390px the prompt, the field and the two verbs stack, and the progress
 *  line has to stay on one row above them. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: interview(form(ANSWERS)),
};
