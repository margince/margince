// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { Badge } from "./atoms";
import {
  type LinkedProject,
  ProjectLinks,
  type ProjectLinksAdapter,
} from "./projectlinks";
import type { RecordPickerCandidate } from "./recordpicker";

const meta: Meta<typeof ProjectLinks> = {
  title: "Design System/Project links",
  component: ProjectLinks,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ProjectLinks>;

// The phase arrives as a NODE, which is the whole reason the prop is one: the
// badge that renders a project phase belongs to the projects screen, and this
// tier may not reach into a screen. A caller hands in whatever it already
// draws; here that is a plain Badge.
const LINKED: readonly LinkedProject[] = [
  {
    project_id: "p-1",
    name: "Warehouse rollout",
    key: "WH-14",
    phase: <Badge tone="accent">Delivery</Badge>,
  },
  {
    project_id: "p-2",
    name: "Q3 data migration",
    key: "DM-7",
    phase: <Badge>Discovery</Badge>,
  },
];

const CATALOG: readonly RecordPickerCandidate[] = [
  { id: "p-3", name: "Billing consolidation" },
  { id: "p-4", name: "Warehouse phase two" },
  { id: "p-5", name: "Security review" },
];

function adapterFor(
  over: Partial<ProjectLinksAdapter> = {},
): ProjectLinksAdapter {
  return {
    linked: LINKED,
    allowsMany: true,
    search: (query) =>
      Promise.resolve(
        CATALOG.filter((candidate) =>
          candidate.name.toLowerCase().includes(query.toLowerCase()),
        ),
      ),
    roles: [
      { value: "customer", label: "Customer" },
      { value: "subcontractor", label: "Subcontractor" },
      { value: "sponsor", label: "Sponsor" },
    ],
    attach: () => Promise.resolve(),
    detach: () => Promise.resolve(),
    ...over,
  };
}

function Live(props: Readonly<{ over?: Partial<ProjectLinksAdapter> }>) {
  const [links, setLinks] = useState<readonly LinkedProject[]>(
    props.over?.linked ?? LINKED,
  );
  return (
    <ProjectLinks
      adapter={adapterFor({
        ...props.over,
        linked: links,
        detach: (projectID) => {
          setLinks((current) =>
            current.filter((link) => link.project_id !== projectID),
          );
          return Promise.resolve();
        },
      })}
      titleKey="companyProjects.title"
      emptyBody="companyProjects.empty"
    />
  );
}

/** The ordinary case: a record on several projects, each nameable, phased and
 * openable, with the verb to attach another. */
export const Linked: Story = { render: () => <Live /> };

/**
 * None yet. The empty body says how a link of this kind comes to EXIST — a bare
 * "nothing here" tells a reader only what they can already see.
 */
export const Empty: Story = { render: () => <Live over={{ linked: [] }} /> };

/**
 * A record that carries at most one, which is a deal. The adapter answers
 * `allowsMany: false` once it has its one, and the verb becomes "move" rather
 * than "attach" — the same section, one truthful word apart.
 */
export const HoldsOnlyOne: Story = {
  render: () => <Live over={{ linked: [LINKED[0]], allowsMany: false }} />,
};

/**
 * Read-only, which is the RECORD's answer rather than each project's. The verb
 * is gone because the reader may not write this record at all.
 */
export const ReadOnly: Story = {
  render: () => <Live over={{ readOnly: true }} />,
};

/**
 * A link that cannot be removed from HERE. `detach` absent is not the same as a
 * refusal: the project's own company list is where that edge comes off, and the
 * row says so by offering no verb rather than by offering one that fails.
 */
export const NotDetachableHere: Story = {
  render: () => (
    <ProjectLinks
      adapter={adapterFor({ detach: undefined })}
      titleKey="companyProjects.title"
      emptyBody="companyProjects.empty"
    />
  ),
};
