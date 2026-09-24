// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import type { WorklistItem } from "./worklist.queries";
// The row's own line. `.worklist-row-acts` carries the trailing alignment and
// the interval between the two answers; without it they draw as a bare inline
// run with no gap, which is a picture of markup rather than of the row.
import "./worklist.row.css";

// ANSWERING AN UNDECIDED DOMAIN, from the row that asked.
//
// TWO VERBS OF EQUAL WEIGHT, so neither is filled and neither takes the end of
// the line: the machine could not tell whether this domain is a company, two
// colleagues sharing an installation may answer it opposite ways, and drawing
// one as the expected press would be the same guess that left the question open.
// That is what these frames are for, and it is invisible to an assertion — both
// buttons render either way.
//
// They are a fragment with no container of their own, so the host is the row's
// line and nothing else, the frame `worklist.dispositions.stories.tsx` gives the
// judgements they share it with. The DOMAIN is the row's id: an open question is
// identified by the domain itself, the disposition row's own id never reaching a
// client.

const UNDECIDED: WorklistItem = {
  id: "mckinsey.com",
  source: "domain_question",
  category: "decisions",
  level: 6,
  consequence: "data_drifts",
  title: "mckinsey.com",
  detail:
    "Nothing on the site named a company, and the sender's name did not explain the domain.",
  because: [],
  actions: ["keep", "discard"],
};

const KEEP = `POST /capture/domain-questions/${UNDECIDED.id}/keep`;
const DISCARD = `POST /capture/domain-questions/${UNDECIDED.id}/discard`;

// Both writes, answered at once. They go to two stores under two gates — keeping
// creates the company the triage withheld, discarding writes one seat's own
// capture exclusion — so a frame routing one proves nothing about the other.
function settleAtOnce() {
  installFetchStub({
    [KEEP]: () => jsonResponse(null, 202),
    [DISCARD]: () => jsonResponse(null, 202),
  });
}

function neverSettles() {
  installFetchStub({ [KEEP]: () => new Promise<Response>(() => {}) });
}

function refused() {
  installFetchStub({
    [KEEP]: () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/conflict",
          title: "Conflict",
          status: 409,
          code: "conflict",
          detail: "A company already holds that domain.",
        },
        409,
      ),
  });
}

const meta: Meta<typeof DomainQuestionAnswer> = {
  title: "Records/Worklist/Domain question verbs",
  component: DomainQuestionAnswer,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DomainQuestionAnswer>;

function line(item: WorklistItem, stub: () => void) {
  return () => {
    stub();
    return (
      <StoryProviders>
        <ToastProvider>
          <Panel title="Unreviewed domain">
            <PanelBody>
              <p className="t-caption">{item.detail}</p>
              <div className="worklist-row-acts">
                <DomainQuestionAnswer item={item} />
              </div>
            </PanelBody>
          </Panel>
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

const pressKeep = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Create company" }),
  );
};

/** Both answers, neither promoted: the pair a reader meets. */
export const TwoAnswersOfEqualWeight: Story = {
  render: line(UNDECIDED, settleAtOnce),
};

/**
 * EACH BUTTON ASKS WHETHER THE ROW OFFERS IT, rather than the pair being drawn
 * because the source is this one. A reader who may exclude the domain for
 * themselves and may not create a company is a real case, and a control drawn
 * past the server's offer is a button that 403s.
 */
export const OnlyTheAnswerTheSeatMayGive: Story = {
  render: line({ ...UNDECIDED, actions: ["discard"] }, settleAtOnce),
};

/**
 * One answer being written, and BOTH standing down. They are two answers to one
 * question, so a colleague who presses both has filed a company and an exclusion
 * for the same domain. The verb pressed is the busy one; the other is merely
 * disabled, because it started nothing.
 */
export const AnAnswerBeingWritten: Story = {
  render: line(UNDECIDED, neverSettles),
  play: pressKeep,
};

/**
 * A refused answer says so, in the product's own words rather than the server's.
 * The row is unchanged and both verbs come back live: the question is still
 * open, and the reader's next press is the retry.
 */
export const TheCompanyCouldNotBeCreated: Story = {
  render: line(UNDECIDED, refused),
  play: pressKeep,
};
