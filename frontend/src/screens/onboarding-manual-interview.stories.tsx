// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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
// walks forward has to have answered at least those, or `advance` refuses
// silently and the story stops on a question its own name does not claim.
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

// Advancing needs the current question ANSWERED when it is a required one, so
// the stories that walk forward start from a filled form. Named from the
// catalog rather than restated: the verb has been reworded before, and a story
// that failed for the wording would be reporting on the copy rather than on
// the step.
async function advance(canvasElement: HTMLElement, times: number) {
  const canvas = within(canvasElement);
  const user = userEvent.setup();
  for (let step = 0; step < times; step += 1) {
    await user.click(
      await canvas.findByRole("button", { name: en["ob.manualNext"] }),
    );
  }
}

// To the END, rather than by a count of steps. A number here is a second
// statement of how many questions there are, and it goes wrong the day one is
// added — which is how this story first failed, looking for a verb that had
// already become "Review my answers". The condition cannot: the run is over
// exactly when the next-question verb is gone.
async function advanceToLast(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const user = userEvent.setup();
  for (;;) {
    const next = canvas.queryByRole("button", { name: en["ob.manualNext"] });
    if (!next) {
      return;
    }
    await user.click(next);
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
  play: async ({ canvasElement }) => advance(canvasElement, 6),
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
