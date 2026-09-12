// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "@tanstack/react-query";
import { Button } from "../design-system/atoms";
import { QueryGate, WriteRefused } from "./common";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The two surfaces `common.tsx` renders for every screen. QueryGate is the
// shared loading/error/empty/loaded ladder a detail read goes through —
// exercised off a small demo query against the shared fetch stub, one story per
// state, rather than a hand-built UseQueryResult (react-query's result shape
// isn't meant to be constructed by hand). WriteRefused is the other half of the
// same honesty: what a screen says when the server refused the write.
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
 *  it. Danger is the only tone it has — a blocked write is never milder. */
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
        actions={<Button small>Retry</Button>}
      />
    </StoryProviders>
  ),
};
