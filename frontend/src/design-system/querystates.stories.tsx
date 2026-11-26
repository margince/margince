// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { Button, Field, Modal, TextInput } from "./atoms";
import { Heading } from "./heading";
import { QueryGate, WriteRefused } from "./querystates";

// The two surfaces `querystates.tsx` renders for every screen. QueryGate is the
// shared loading/error/empty/loaded ladder a detail read goes through.
// Each state gets one story, driven by a small demo query against the shared
// fetch stub. React-query's result shape isn't meant to be built by hand.
// WriteRefused is the other half: what a screen says when the server refused the write.
const meta: Meta = {
  title: "Patterns/Screen plumbing",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Demo({ empty }: Readonly<{ empty?: boolean }>) {
  const query = useQuery({
    queryKey: ["story-demo"],
    queryFn: async () => {
      // contract-fetch:allow story fixture — /v1/story-demo is no contract operation; the story needs a query that fails without a stack behind it
      const response = await fetch("/v1/story-demo");
      const body = (await response.json()) as { name: string } | null;
      if (!response.ok) {
        throw new Error("request failed");
      }
      return body;
    },
  });
  return (
    <QueryGate
      query={query}
      empty={empty ? () => true : undefined}
      pendingLabel="Loading the section"
    >
      {(data) => <p>{data?.name}</p>}
    </QueryGate>
  );
}

export const Pending: Story = {
  render: () => {
    installFetchStub({
      "GET /story-demo": () => new Promise<Response>(() => undefined),
    });
    return (
      <StoryProviders>
        <Demo />
      </StoryProviders>
    );
  },
};

export const ErrorState: Story = {
  render: () => {
    installFetchStub({
      "GET /story-demo": () =>
        jsonResponse(
          { title: "Forbidden", detail: "missing scope contacts:read" },
          403,
        ),
    });
    return (
      <StoryProviders>
        <Demo />
      </StoryProviders>
    );
  },
};

export const EmptyState: Story = {
  render: () => {
    installFetchStub({
      "GET /story-demo": () => jsonResponse({ name: "unused" }),
    });
    return (
      <StoryProviders>
        <Demo empty />
      </StoryProviders>
    );
  },
};

export const Loaded: Story = {
  render: () => {
    installFetchStub({
      "GET /story-demo": () => jsonResponse({ name: "Anna Weber" }),
    });
    return (
      <StoryProviders>
        <Demo />
      </StoryProviders>
    );
  },
};

/** A refused write: the screen's claim as the heading, the server's cause under
 *  it. Danger is the only tone it has: a blocked write is never milder. */
export const Refused: Story = {
  render: () => (
    <StoryProviders>
      <WriteRefused
        titleKey="settings.saveFailed"
        message="That signature is longer than the 2000 characters allowed."
      />
    </StoryProviders>
  ),
};

/** The same refusal where the reader has a move: the verb sits at the end of
 *  the heading's line, and drops under the body on a phone. */
export const RefusedWithRetry: Story = {
  render: () => (
    <StoryProviders>
      <WriteRefused
        titleKey="settings.saveFailed"
        message="The connection dropped before the save landed."
        actions={<Button>Retry</Button>}
      />
    </StoryProviders>
  ),
};

const signatureFields = [
  "Display name",
  "Job title",
  "Phone",
  "Mobile",
  "Office address",
  "Website",
  "Booking link",
  "Sign-off",
  "Closing line",
  "Legal notice",
];

function SignatureDialog() {
  const titleId = useId();
  const [refused, setRefused] = useState<string | null>(null);
  return (
    <StoryProviders>
      <Modal open onClose={() => undefined} labelledBy={titleId} intent="form">
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          Edit signature
        </Heading>
        <div className="form-stack">
          {signatureFields.map((label) => (
            <Field key={label} label={label}>
              {(control) => <TextInput {...control} />}
            </Field>
          ))}
          <WriteRefused titleKey="settings.saveFailed" message={refused} />
        </div>
        <div className="actions">
          <Button>Cancel</Button>
          <Button
            variant="primary"
            onClick={() =>
              setRefused(
                "That signature is longer than the 2000 characters allowed.",
              )
            }
          >
            Save
          </Button>
        </div>
      </Modal>
    </StoryProviders>
  );
}

/** A refusal that lands below the fold of a dialog scrolls up to stand above
 *  the pinned action row, so the reader sees it arrive. */
export const RefusedInADialog: Story = {
  name: "Refused inside a dialog",
  render: () => <SignatureDialog />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = await screen.findByRole("dialog", {
      name: "Edit signature",
    });
    const actions = dialog.querySelector<HTMLElement>(":scope > .actions");
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    const alert = await within(dialog).findByRole("alert");
    await waitFor(
      () =>
        expect(alert.getBoundingClientRect().bottom).toBeLessThanOrEqual(
          (actions?.getBoundingClientRect().top ?? 0) + 1,
        ),
      { timeout: 3000 },
    );
  },
};
