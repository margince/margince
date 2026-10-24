// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen } from "storybook/test";
import {
  type PickableProject,
  ProjectPicker,
} from "../../design-system/projectpicker";
import { StoryProviders } from "../../testing/story-utils";
import { stripSubjectTag, withSubjectTag } from "../projects/projectrecord";
import { ReplyStandingNotice, SubjectTagSwitch } from "./compose.head";
import { SubjectRow } from "./composehead";

const KEY = "[N2P-1]";
const PROJECTS: PickableProject[] = [
  {
    project_id: "pr-9",
    name: "Netcare 2 project",
    key: "N2P-1",
    phase: "delivering",
  },
];
const LONG_SUBJECT =
  "Re: Angebot für die Erweiterung der Lagerverwaltung, zweite Ausbaustufe mit Schnittstelle zum ERP";
const NAMES: Record<string, string> = {
  "u-charlotte": "Charlotte Weber",
  "u-long": "Maximiliane Hohenstein-Wittelsbach von Brandenburg",
  "u-long-2": "Nguyễn Thị Phương Thảo Bảo Ngọc",
};
const nameOf = (_type: string, id: string) => NAMES[id];
const SWITCH = { name: "Add project key to subject (helps filing)" };

function Head({
  initialSubject = "Re: Angebot",
  seats = [],
  offThread = false,
  owner,
}: Readonly<{
  initialSubject?: string;
  seats?: readonly string[];
  offThread?: boolean;
  owner?: string;
}>) {
  const [subject, setSubject] = useState(withSubjectTag(initialSubject, KEY));
  const [tagged, setTagged] = useState(true);
  const turn = (next: boolean) => {
    setTagged(next);
    setSubject((current) =>
      next ? withSubjectTag(current, KEY) : stripSubjectTag(current, KEY),
    );
  };
  return (
    <StoryProviders>
      <div className="form-stack">
        <ReplyStandingNotice
          seats={seats}
          offThread={offThread}
          owner={owner}
          nameOf={nameOf}
        />
        <SubjectRow
          subject={subject}
          onChange={setSubject}
          invalid={false}
          need="Enter a subject."
          aside={
            <ProjectPicker
              projects={PROJECTS}
              projectId="pr-9"
              onChange={() => {}}
              compact
            />
          }
          under={
            <SubjectTagSwitch
              filing={{
                projectId: "pr-9",
                setProjectId: () => {},
                ownTag: KEY,
                tagged,
                setTagged: turn,
              }}
              disabled={false}
            />
          }
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Patterns/Compose mail/Subject and reply notice",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** The key goes out by default; turning it off takes it out of the subject. */
export const KeySwitch: Story = {
  render: () => <Head />,
  play: async () => {
    const key = await screen.findByRole("switch", SWITCH);
    await expect(key).toHaveAttribute("aria-checked", "true");
    await expect(screen.getByRole("textbox", { name: "Subject" })).toHaveValue(
      "[N2P-1] Re: Angebot",
    );
  },
};

/** At phone width the switch takes its own line under a long subject. */
export const KeySwitchPhone: Story = {
  render: () => <Head initialSubject={LONG_SUBJECT} />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    await expect(await screen.findByRole("switch", SWITCH)).toBeVisible();
  },
};

/** A colleague's mailbox took the thread; the reply goes to the To line. */
export const ColleagueMailbox: Story = {
  render: () => <Head seats={["u-charlotte"]} />,
  play: async () => {
    await screen.findByRole("switch", SWITCH);
    await expect(
      screen.getByText(/came in to Charlotte Weber’s mailbox/),
    ).toBeVisible();
  },
};

/** The reader is not on the thread, and the deal owner is named. */
export const NotOnThread: Story = {
  render: () => (
    <Head seats={["u-charlotte"]} offThread owner="Charlotte Weber" />
  ),
  play: async () => {
    await screen.findByRole("switch", SWITCH);
    await expect(
      screen.getByText(
        "You are not on this thread. Charlotte Weber owns this deal.",
      ),
    ).toBeVisible();
  },
};

/** Nobody's mailbox took it, so the off-thread fact stands on its own. */
export const NotOnThreadAlone: Story = {
  render: () => <Head offThread owner="Charlotte Weber" />,
};

/** A deal with no owner the roster can name says only the first fact. */
export const NotOnThreadOwnerUnknown: Story = {
  render: () => <Head seats={["u-charlotte"]} offThread />,
};

/** Long names in a phone-wide head: every line wraps, nothing is cut. */
export const LongNamesPhone: Story = {
  render: () => (
    <Head
      initialSubject={LONG_SUBJECT}
      seats={["u-long", "u-long-2"]}
      offThread
      owner={NAMES["u-long"]}
    />
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    await screen.findByRole("switch", SWITCH);
    await expect(
      screen.getByText(/Maximiliane Hohenstein-Wittelsbach .* owns this deal/),
    ).toBeVisible();
  },
};
