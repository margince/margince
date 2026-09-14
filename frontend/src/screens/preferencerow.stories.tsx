// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useT } from "../i18n";
import { PreferenceRow } from "./preferencerow";
import type { PurposeView } from "./preferences.logic";
import { StoryProviders } from "./story-utils";
import "./preferences.css";

// One purpose on the public preference centre, outside the page that queries
// it. The states worth a picture are the ones the row decides on its own: a
// locked lane, which carries the "always on" badge with its lock and cannot be
// switched; an ordinary lane a subject may turn off; and a lane that needs a
// confirmation link before a grant counts. The page with every row in it is
// `Signed out/Email preference centre`.

function purpose(over: Partial<PurposeView>): PurposeView {
  return {
    key: "marketing_email",
    label: "Product updates",
    state: "granted",
    locked: false,
    choice: "opted_in",
    can_opt_in: true,
    grant_needs_confirmation: false,
    ...over,
  };
}

function Rows({
  rows,
}: Readonly<{ rows: { purpose: PurposeView; on: boolean }[] }>) {
  const t = useT();
  return (
    <ul className="pref-list">
      {rows.map((row) => (
        <PreferenceRow
          key={row.purpose.key}
          purpose={row.purpose}
          on={row.on}
          wording={row.purpose.label}
          t={t}
          onToggle={() => {}}
          disabled={false}
          mayGrant
        />
      ))}
    </ul>
  );
}

const meta: Meta<typeof PreferenceRow> = {
  title: "Signed out/Email preference centre/Purpose row",
  component: PreferenceRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PreferenceRow>;

/** A locked transactional lane above an ordinary one. */
export const LockedAndOpen: Story = {
  render: () => (
    <StoryProviders>
      <Rows
        rows={[
          {
            purpose: purpose({
              key: "transactional",
              label: "Deal & service messages",
              locked: true,
              choice: "no_objection",
              can_opt_in: false,
            }),
            on: true,
          },
          { purpose: purpose({}), on: true },
        ]}
      />
    </StoryProviders>
  ),
};

/** Switched off, and a grant needs the confirmation link to count. */
export const NeedsConfirmation: Story = {
  render: () => (
    <StoryProviders>
      <Rows
        rows={[
          {
            purpose: purpose({
              key: "events",
              label: "Events",
              state: "unknown",
              choice: "opted_out",
              can_opt_in: false,
              grant_needs_confirmation: true,
            }),
            on: false,
          },
        ]}
      />
    </StoryProviders>
  ),
};
