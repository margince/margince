import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { UTC_ZONE } from "../format/timezone";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import {
  type DeadlineDraft,
  DsrDeadlineFields,
  EMPTY_DEADLINE,
  useRequestDeadline,
} from "./privacy.deadline";

// The deadline fields of a manual subject request: the arrival day, the
// deadline the server computes with its rule, and an optional earlier date.

function Harness({ initial }: Readonly<{ initial: DeadlineDraft }>) {
  const [draft, setDraft] = useState(initial);
  const query = useRequestDeadline("access", draft, UTC_ZONE);
  return (
    <form className="form-stack">
      <DsrDeadlineFields
        draft={draft}
        query={query}
        refused={false}
        zone={UTC_ZONE}
        onChange={(next) => setDraft((was) => ({ ...was, ...next }))}
      />
    </form>
  );
}

function story(rule: string, dueAt: string, initial = EMPTY_DEADLINE) {
  return () => {
    stubWithSession(
      {
        "GET /data-subject-requests/deadline": () =>
          jsonResponse({ due_at: dueAt, rule }),
      },
      { privacy_request: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <Harness initial={initial} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DsrDeadlineFields> = {
  title: "Settings/Governance/Privacy and retention/Request deadline",
  component: DsrDeadlineFields,
};
export default meta;
type Story = StoryObj<typeof DsrDeadlineFields>;

/** No jurisdiction pack sets a deadline: the GDPR month. */
export const GdprMonth: Story = {
  render: story("gdpr", "2026-11-11T09:00:00Z"),
};
export const GdprMonthDark: Story = {
  ...GdprMonth,
  globals: { theme: "dark" },
};

/** A pack's own rule set, counted in hours. */
export const PackRule: Story = {
  render: story("zz v1", "2026-10-14T09:00:00Z"),
};

/** An earlier date past the computed deadline, which the form refuses. */
export const EarlierDateTooLate: Story = {
  render: story("gdpr", "2026-11-11T09:00:00Z", {
    receivedOn: "",
    earlierDue: "2026-11-20",
  }),
};
export const EarlierDateTooLateDark: Story = {
  ...EarlierDateTooLate,
  globals: { theme: "dark" },
};
