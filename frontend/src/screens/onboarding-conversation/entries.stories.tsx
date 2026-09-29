import type { Meta, StoryObj } from "@storybook/react-vite";
import { MarginceCoreScene } from "../../design-system/margince-core";
import { LocaleProvider } from "../../i18n";
import {
  type ConversationState,
  initialConversationState,
  type ThreadEntry,
} from "./conversation-machine";
import { NarrationBubble } from "./entries";
import { presenceFor } from "./presence";

const meta: Meta = {
  title: "Onboarding/Conversation/Entries",
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

const narration: Extract<ThreadEntry, { kind: "narration" }> = {
  kind: "narration",
  id: "1:field:industry",
  i18nKey: "ob.conv.read.learnedField",
  params: { value: "Industrial robotics" },
  paramKeys: { field: "ob.field.industry" },
  findingIds: ["industry"],
};

export const Narration: Story = {
  render: () => <NarrationBubble entry={narration} />,
};

// The live-arrival treatment: words stagger in (capped total duration);
// under prefers-reduced-motion the sentence is simply there.
export const NarrationRevealed: Story = {
  render: () => <NarrationBubble entry={narration} reveal />,
};

// The orb choreography at a glance: one presence per conversation moment,
// derived through the same presenceFor mapping the acts use.
const choreography: ReadonlyArray<{
  label: string;
  state: ConversationState;
  read?: Parameters<typeof presenceFor>[1];
}> = [
  {
    label: "co.intro",
    state: { ...initialConversationState, act: "company", phase: "co.intro" },
  },
  {
    label: "co.reading",
    state: { ...initialConversationState, act: "company", phase: "co.reading" },
    read: { read: { status: "reading", phase: "crawling", pages_read: 14 } },
  },
  {
    label: "co.clarify",
    state: { ...initialConversationState, act: "company", phase: "co.clarify" },
  },
  {
    label: "co.review",
    state: { ...initialConversationState, act: "company", phase: "co.review" },
  },
  {
    label: "vo.building",
    state: {
      ...initialConversationState,
      act: "voice",
      phase: "vo.building",
      lastBuildStage: "evaluate",
    },
  },
  {
    label: "vo.result failed",
    state: {
      ...initialConversationState,
      act: "voice",
      phase: "vo.result",
      lastBuildStatus: "failed",
    },
  },
  {
    label: "done",
    state: { ...initialConversationState, act: "done", phase: "done" },
  },
];

export const OrbChoreography: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(4, 1fr)",
        gap: "var(--space-4)",
      }}
    >
      {choreography.map((moment) => {
        const presence = presenceFor(moment.state, moment.read);
        return (
          <figure key={moment.label} style={{ margin: 0, textAlign: "center" }}>
            <MarginceCoreScene
              state={presence.core}
              progress={presence.progress}
            />
            <figcaption className="t-caption">{moment.label}</figcaption>
          </figure>
        );
      })}
    </div>
  ),
};
