import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { StoryProviders } from "../../testing/story-utils";
import type { AccountScan } from "./accountscan";
import { scanFoot } from "./companytoday.lines";

function ScanReceipt({ reason }: Readonly<{ reason: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const scan: AccountScan = {
    company_id: "01a04298-1971-7076-8076-8064da20fdff",
    state: "queued",
    findings: [],
    findings_dropped: 0,
    resumes_at: "2026-08-01T06:00:00Z",
    degrade_reason: reason,
  };
  return scanFoot({ scan, t, plural, locale, recordZone });
}

const meta: Meta<typeof ScanReceipt> = {
  title: "Records/Company 360/Scan receipt",
  component: ScanReceipt,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof meta>;

export const WaitingForBudget: Story = { args: { reason: "budget_deferred" } };
export const WaitingForProvider: Story = {
  args: { reason: "provider_deferred" },
};
export const ScheduledRetry: Story = { args: { reason: "future_reason" } };
export const WaitingForProviderDark: Story = {
  ...WaitingForProvider,
  globals: { theme: "dark" },
};
