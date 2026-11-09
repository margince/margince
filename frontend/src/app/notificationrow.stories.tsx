import type { Meta, StoryObj } from "@storybook/react-vite";
import { OpenSourceProvider } from "../lib/opensource";
import { StoryProviders } from "../testing/story-utils";
import { NoticeRow } from "./notificationrow";

const meta: Meta<typeof NoticeRow> = {
  title: "Shell/Notification row",
  component: NoticeRow,
  decorators: [
    (Story) => (
      <StoryProviders>
        <OpenSourceProvider openSource={() => undefined}>
          <ul className="notifbell-list">
            <Story />
          </ul>
        </OpenSourceProvider>
      </StoryProviders>
    ),
  ],
  args: {
    notice: {
      id: "notice-1",
      kind: "automation_failed",
      subject: "An automation could not run",
      created_at: "2026-09-15T09:00:00Z",
      target: { type: "deal", id: "22222222-2222-4222-8222-222222222222" },
    },
    onSettle: () => {},
    onActivate: () => {},
    settling: false,
  },
};
export default meta;
type Story = StoryObj<typeof NoticeRow>;
export const Unread: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
export const Settling: Story = { args: { settling: true } };

const privacyNotice = {
  id: "privacy-review",
  kind: "privacy_review_required",
  subject: "Manual privacy review needed",
  body: "This retained note may mention an erased contact. Review the possible match manually; its text was kept.",
  created_at: "2026-10-01T12:00:00Z",
  target: { type: "activity", id: "11111111-1111-4111-8111-111111111111" },
};
export const RetainedNote: Story = { args: { notice: privacyNotice } };
export const RetainedNoteDark: Story = {
  ...RetainedNote,
  globals: { theme: "dark" },
};
export const UploadedFile: Story = {
  args: {
    notice: {
      ...privacyNotice,
      body: "An uploaded document may mention an erased contact. Keep the file and open its original to review the text.",
      target: {
        type: "knowledge_document",
        id: "22222222-2222-4222-8222-222222222222",
      },
    },
  },
};
export const OwnSignature: Story = {
  args: {
    notice: {
      ...privacyNotice,
      body: "An email signature may mention an erased contact. Review the signature before deciding what to remove.",
      target: {
        type: "email_signature",
        id: "33333333-3333-4333-8333-333333333333",
      },
    },
  },
};
