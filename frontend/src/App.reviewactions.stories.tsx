import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReviewActionsProvider } from "./App.reviewactions";
import { sheetLabels } from "./features/analytics/analytics.forecast.review";
import { InputCheckAnswerSheet } from "./features/analytics/inputcheck.answer";
import { useT } from "./i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./testing/story-utils";

const meta = {
  title: "Records/Reports/Forecast/Buying evidence",
  component: InputCheckAnswerSheet,
} satisfies Meta<typeof InputCheckAnswerSheet>;
export default meta;
type Story = StoryObj;
function Answer({
  historical = false,
  buyerVerified = true,
  missingCompany = false,
}: {
  historical?: boolean;
  buyerVerified?: boolean;
  missingCompany?: boolean;
}) {
  const t = useT();
  return (
    <InputCheckAnswerSheet
      open
      labels={sheetLabels(t)}
      onClose={() => {}}
      onSubmit={() => {}}
      check={{
        id: "finding-one",
        type: "unlinked_signal",
        subject_kind: "signal",
        subject_id: "signal-one",
        subject: { type: "company", id: "company-one", label: "Acme" },
        claim: missingCompany ? undefined : { company_id: "company-one" },
        observed: {
          suggestion_id: missingCompany ? "suggestion-one" : undefined,
          occurred_at: "2026-07-01T10:00:00Z",
          historical,
          buyer_verified: buyerVerified,
        },
        severity: "low",
        status: "open",
        first_seen_at: "2026-07-02T10:00:00Z",
        inputs_changed: false,
        last_seen_at: "2026-07-02T10:00:00Z",
      }}
    />
  );
}
function renderAnswer(
  historical: boolean,
  buyerVerified = true,
  mayAnswer = true,
  missingCompany = false,
) {
  installFetchStub({
    "GET /me": meRoute({
      deal: ["read", "update"],
      signal: mayAnswer ? ["read", "update"] : ["read"],
      forecast: ["read", "update"],
    }),
    "GET /signals/signal-one": () =>
      jsonResponse({
        summary: "The buyer asked for a support proposal",
        evidence: [
          { snippet: "Please send us a proposal for annual support." },
        ],
      }),
    "GET /deals": () =>
      jsonResponse({
        data: [{ id: "support", name: "Annual support" }],
        page: { has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <ReviewActionsProvider>
        <Answer
          historical={historical}
          buyerVerified={buyerVerified}
          missingCompany={missingCompany}
        />
      </ReviewActionsProvider>
    </StoryProviders>
  );
}
export const CurrentEvidence: Story = { render: () => renderAnswer(false) };
export const HistoricalEvidence: Story = { render: () => renderAnswer(true) };

export const BuyerNeedsConfirmation: Story = {
  render: () => renderAnswer(false, false),
};

export const ReadOnlySignal: Story = {
  render: () => renderAnswer(false, true, false),
};

export const CoveredFindingLostCompany: Story = {
  render: () => renderAnswer(false, true, true, true),
};
