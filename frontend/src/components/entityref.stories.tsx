// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  storyQueryClient,
} from "../testing/story-utils";
import { EntityRef } from "./entityref";
import { companyReferenceQuery } from "./entityref.queries";

const meta: Meta<typeof EntityRef> = {
  title: "Patterns/Entity reference",
  component: EntityRef,
  parameters: { layout: "padded" },
  decorators: [
    (Story, { args }) => {
      const client = storyQueryClient();
      if (args.id === "o-cached") {
        client.setQueryData(companyReferenceQuery("o-cached").queryKey, {
          name: "Cached Company",
          archived: false,
          logo: null,
        });
      }
      installFetchStub({
        "GET /records/references": () => {
          if (args.id === "o-cached") {
            return jsonResponse({ title: "Unexpected company read" }, 500);
          }
          if (args.id === "o-refused") {
            return jsonResponse({ title: "permission denied" }, 403);
          }
          return jsonResponse({
            data:
              args.id === "o-1"
                ? [
                    {
                      id: "o-1",
                      name: "Brandt Automotive GmbH",
                      archived: false,
                      logo: null,
                    },
                  ]
                : args.id === "ct-1"
                  ? [{ id: "ct-1", name: "Anna Weber", archived: true }]
                  : args.id === "l-email"
                    ? [
                        {
                          id: "l-email",
                          name: "lead@example.com",
                          archived: false,
                        },
                      ]
                    : [],
          });
        },
        // A roster that offers another cursor for every page it answers: the
        // walk spends its page budget and still has not reached the end, which
        // is the state the fourth reading exists for.
        "GET /users": () =>
          jsonResponse({
            data: [{ id: "u-1", display_name: "Priya Shah" }],
            page: { next_cursor: "more", has_more: true },
          }),
      });
      return (
        <StoryProviders client={client}>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof EntityRef>;

export const ResolvedBacklink: Story = {
  args: { kind: "company", id: "o-1" },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("link", {
        name: "Brandt Automotive GmbH",
      }),
    ).resolves.toBeVisible();
  },
};

export const CachedCompanyReference: Story = {
  args: { kind: "company", id: "o-cached" },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("link", {
        name: "Cached Company",
      }),
    ).resolves.toBeVisible();
  },
};

export const UnresolvedFallsBackToId: Story = {
  args: { kind: "company", id: "o-unknown" },
};

export const FailedReadSaysSo: Story = {
  args: { kind: "company", id: "o-refused" },
};

export const RosterStoppedShortSaysNameDidNotLoad: Story = {
  args: { kind: "user", id: "u-past-the-walk" },
};

export const RecordCardDestination: Story = {
  args: {
    kind: "deal",
    id: "01a00000-0000-7000-8000-000000000001",
    name: "Example renewal · 海",
  },
};

export const ArchivedContactReference: Story = {
  args: { kind: "contact", id: "ct-1" },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("link", { name: "Anna Weber" }),
    ).resolves.toBeVisible();
  },
};

export const LeadReferenceWithEmailName: Story = {
  args: { kind: "lead", id: "l-email" },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("link", { name: "lead@example.com" }),
    ).resolves.toBeVisible();
  },
};
