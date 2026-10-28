// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Modal } from "../design-system/atoms";
import { DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import {
  ANA,
  contactCase,
  previewOutcome,
  resolvedCase,
} from "../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { ContactCaseReview } from "./contactcase";
import { CasePreview } from "./contactcase.evidence";
import { CaseForm } from "./contactcase.form";
import type { ContactCase } from "./contactcase.queries";

// The review of a group of contacts that may be one person, in each state a
// reader can meet it in.
const meta: Meta = { title: "Patterns/Contact duplicate review" };
export default meta;

type Story = StoryObj;

function DelegatedForm() {
  const headingId = useId();
  return (
    <StoryProviders>
      <Modal
        open
        onClose={() => undefined}
        labelledBy={headingId}
        intent="drawer"
      >
        <DrawerHead>
          <Heading size="large" id={headingId} className="t-h2 modal-title">
            Possible duplicates
          </Heading>
        </DrawerHead>
        <CaseForm
          contactCase={threeMembers({
            policy_enabled: true,
            merge_mode: "delegated",
          })}
          startId={ANA}
          stale={false}
          onClose={() => undefined}
          onStale={() => undefined}
          onMerged={() => undefined}
        />
      </Modal>
    </StoryProviders>
  );
}

const CASE_PATH = `/dedupe/contact-cases/${ANA}`;

// The first three members only: Ana, Ben and Cleo, joined by two open pairs.
function threeMembers(over: Partial<ContactCase> = {}): ContactCase {
  const full = contactCase();
  return contactCase({
    members: full.members.slice(0, 3),
    edges: full.edges.slice(0, 2),
    owners: full.owners.slice(0, 3),
    ...over,
  });
}

function review(
  answer: () => Response,
  extra: Record<string, () => Response> = {},
) {
  stubWithSession({ [`GET ${CASE_PATH}`]: answer, ...extra }, {});
  return (
    <StoryProviders>
      <ContactCaseReview contactId={ANA} open onClose={() => undefined} />
    </StoryProviders>
  );
}

/** Three members, all ticked, Ana's record kept and Ana owning it. */
export const ThreeMembers: Story = {
  render: () => review(() => jsonResponse(threeMembers())),
};

/**
 * A fourth member marked as a different person from Ana stays unticked. Its
 * pair offers Undo, and every open pair offers "Not the same person".
 */
export const DismissedPair: Story = {
  render: () => review(() => jsonResponse(contactCase())),
  play: async () => {
    const body = within(document.body);
    await expect(
      await body.findByRole("button", {
        name: "Undo: Mira Kovac and Mira Kovacs",
      }),
    ).toBeVisible();
  },
};

/** The shared merge reviews setting lets an owner of one member merge the colleagues' copies. */
export const Delegated: Story = { render: () => <DelegatedForm /> };

/** No authority over the colleagues' records, and the policy is off. */
export const NotAllowed: Story = {
  render: () =>
    review(() =>
      jsonResponse(
        threeMembers({
          can_merge: false,
          merge_mode: "none",
          reason: "no_write_authority",
        }),
      ),
    ),
};

const tooLarge = () =>
  jsonResponse(
    contactCase({
      can_merge: false,
      merge_mode: "none",
      too_large: true,
      reason: "too_large",
    }),
  );

/** More members than one review may merge: the pairs decide one by one. */
export const TooLarge: Story = { render: () => review(tooLarge) };

/** One pair of a too-large case, asking which of the two contacts stays. */
export const TooLargePairMerge: Story = {
  render: () => review(tooLarge),
  play: async () => {
    const body = within(document.body);
    await userEvent.click(
      await body.findByRole("button", {
        name: "Merge these two: Mira Kovac and Mira Kovač",
      }),
    );
    await expect(
      body.getByRole("button", { name: "Keep Mira Kovač" }),
    ).toBeVisible();
  },
};

/** Someone merged this contact before the reader got to it. */
export const Resolved: Story = {
  render: () => review(() => jsonResponse(resolvedCase())),
};

/** The case changed under the preview, so the review read it again. */
export const Stale: Story = {
  render: () =>
    review(() => jsonResponse(threeMembers()), {
      [`POST ${CASE_PATH}/preview`]: () =>
        jsonResponse(
          { title: "Conflict", status: 409, code: "case_changed" },
          409,
        ),
    }),
  play: async () => {
    const body = within(document.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Preview merge" }),
    );
    await expect(
      await body.findByText(
        "The duplicates changed while you reviewed them. Check the selection again.",
      ),
    ).toBeVisible();
  },
};

/** The preview: Cleo's read seat keeps read access, Ben's inactive account gets none. */
export const Preview: Story = {
  render: () =>
    review(() => jsonResponse(threeMembers()), {
      [`POST ${CASE_PATH}/preview`]: () => jsonResponse(previewOutcome()),
    }),
  play: async () => {
    const body = within(document.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Preview merge" }),
    );
    await expect(await body.findByText("Cleo Reader, can view")).toBeVisible();
  },
};

/** The preview section on its own, as the server answered it. */
export const PreviewSection: Story = {
  render: () => (
    <StoryProviders>
      <CasePreview contactCase={threeMembers()} outcome={previewOutcome()} />
    </StoryProviders>
  ),
};
