// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { EMPTY_DRAFT } from "../onboarding";
import { StoryProviders } from "../story-utils";
import { CompanyActArtifact } from "./artifact";
import "../onboarding.css";
import "./conversation.css";

// The company act's work surface, one scene at a time. The act's own stories
// (Onboarding/Conversation/Company act) drive it through the reducer; these
// mount the pane directly, which is the only way to see a scene the reducer
// reaches by a path several answers long.
//
// The scene these are about is `edit`: the plain field form with the boundary
// sentence above Continue. That sentence is what a reader is agreeing to when
// they press the button beside it, and it is the smallest text on the pane —
// so it is rendered here at its real size rather than only inside the act.

const meta: Meta<typeof CompanyActArtifact> = {
  title: "Onboarding/Conversation/Company artifact",
  component: CompanyActArtifact,
};
export default meta;
type Story = StoryObj<typeof CompanyActArtifact>;

function pane(
  props: Partial<Parameters<typeof CompanyActArtifact>[0]> = {},
  locale?: "de",
) {
  return () => (
    <StoryProviders locale={locale}>
      <CompanyActArtifact
        mode="edit"
        manual={false}
        read={null}
        draft={EMPTY_DRAFT}
        setField={() => {}}
        onPickEntity={() => {}}
        selectedFactKeys={[]}
        setSelectedFactKeys={() => {}}
        missingRequired={[]}
        highlight={null}
        onSwitchMode={() => {}}
        onConfirm={() => {}}
        confirmPending={false}
        confirmDisabled={false}
        saveError={null}
        {...props}
      />
    </StoryProviders>
  );
}

/** The edit escape hatch: the classic form, the boundary sentence, and the
 *  Continue that acts on it. */
export const EditTheFields: Story = { render: pane() };

/** The confirm in flight. The sentence stays put while the button spins — a
 *  boundary a reader has already agreed to must not vanish mid-press. */
export const Confirming: Story = {
  render: pane({ confirmPending: true }),
};

/** A confirm the server refused in a way pressing again cannot change, said on
 *  the surface that carries the button, with the one look that can end the
 *  state beside it. */
export const Refused: Story = {
  render: pane({
    refusal: {
      message:
        "Another colleague confirmed this company while you were editing. Reload to see what they stored.",
      retry: { run: () => {}, busy: false },
    },
  }),
};

/** Nothing read yet, on the dossier scene: the pane says so rather than
 *  showing a dossier the reader is about to leave. */
export const NothingSourcedYet: Story = {
  render: pane({ mode: "dossier" }),
};

/** The German pane, where the boundary sentence runs longest and still has to
 *  read as one line above the verb. */
export const German: Story = { render: pane({}, "de") };

/** The pane in the dark theme. The boundary sentence is the muted role beside
 *  a primary button, which is the pairing the dark accent lift moves. */
export const EditTheFieldsDark: Story = {
  globals: { theme: "dark" },
  render: pane(),
};
