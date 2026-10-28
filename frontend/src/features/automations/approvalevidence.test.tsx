/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { DecisionApproval } from "../../design-system/decisioncard";
import { formatDateTime } from "../../format/format";
import { LocaleProvider, translate } from "../../i18n";
import { RecordZoneProvider } from "../../lib/recordzone";
import { messageApproval } from "../../testing/message-approval";
import { SourceHost } from "../../testing/sourcehost";
import { ApprovalEvidence } from "./approvalevidence";

const SOURCE = "22222222-2222-4222-8222-222222222222";
const RECORD_ZONE = "Pacific/Auckland";
const WHEN = "2026-09-01T09:00:00Z";
const proposal: DecisionApproval = {
  id: "ap1",
  kind: "commitment_task",
  status: "pending",
  proposed_by: "agent:meeting",
  created_at: WHEN,
  summary: "Send the revised scope",
  target_entity_type: "activity",
  target_entity_id: SOURCE,
};
const source = {
  id: SOURCE,
  kind: "meeting",
  subject: "Review meeting",
  body: "Recorded words",
  occurred_at: WHEN,
  is_done: true,
  source: "manual",
  captured_by: "human:reviewer",
  created_at: WHEN,
  updated_at: WHEN,
  links: [],
  version: 1,
};

function show(
  approval = proposal,
  activity: typeof source & { content_state?: string } = source,
  status = 200,
) {
  const fetcher = vi.fn(
    async () =>
      new Response(
        JSON.stringify(
          status === 200
            ? activity
            : { code: status === 403 ? "permission_denied" : "not_found" },
        ),
        {
          status,
          headers: { "content-type": "application/json" },
        },
      ),
  );
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={RECORD_ZONE}>
          <SourceHost>
            <ApprovalEvidence approval={approval} />
          </SourceHost>
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetcher;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the complete proposed message and opens its actual attachment", () => {
  const fetcher = show(messageApproval());
  expect(screen.getByText(/Please confirm acceptance by Friday/)).toBeVisible();
  expect(
    screen.getByRole("link", { name: /Delivery agreement.pdf/ }),
  ).toHaveAttribute(
    "href",
    "/v1/attachments/22222222-2222-4222-8222-222222222222",
  );
  expect(fetcher).not.toHaveBeenCalled();
});

it.each(["unavailable", "stale", "incomplete"])(
  "withholds the message and files when context is %s",
  (state) => {
    const approval = messageApproval();
    if (!approval.preview) throw new Error("message fixture needs a preview");
    const fetcher = show({
      ...approval,
      preview: { ...approval.preview, state },
    });
    expect(
      screen.queryByText(/Please confirm acceptance by Friday/),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Delivery agreement.pdf/ }),
    ).not.toBeInTheDocument();
    expect(fetcher).not.toHaveBeenCalled();
  },
);

it("names and dates the source before opening the existing activity reader", async () => {
  show();
  expect(await screen.findByText(/Review meeting/)).toHaveTextContent(
    formatDateTime(WHEN, "en", RECORD_ZONE),
  );
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("button", { name: translate("en", "tasks.openSource") }),
  );
  expect(await screen.findByRole("dialog")).toHaveTextContent("Recorded words");
});

it("opens a legacy source id without inventing a target", async () => {
  const fetcher = show({
    ...proposal,
    target_entity_type: null,
    target_entity_id: null,
    proposed_change: { source_activity_id: SOURCE },
  });
  expect(await screen.findByText(/Review meeting/)).toBeInTheDocument();
  expect(fetcher.mock.calls.length).toBeGreaterThan(0);
});

it("does not read a source for a different proposal kind", () => {
  const fetcher = show({ ...proposal, kind: "advance_deal" });
  expect(fetcher).not.toHaveBeenCalled();
  expect(
    screen.queryByText(translate("en", "stat.evidence")),
  ).not.toBeInTheDocument();
});

it("withholds the source title and date when its content is withheld", async () => {
  show(proposal, { ...source, content_state: "withheld" });
  expect(
    await screen.findByText(translate("en", "email.detail.withheldReason")),
  ).toBeInTheDocument();
  expect(screen.queryByText(/Review meeting/)).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: translate("en", "tasks.openSource") }),
  ).not.toBeInTheDocument();
});

it("keeps a missing source explicit without making a read", () => {
  const fetcher = show({
    ...proposal,
    target_entity_type: null,
    target_entity_id: null,
  });
  expect(
    screen.getByText(translate("en", "stat.evidence")),
  ).toBeInTheDocument();
  expect(fetcher).not.toHaveBeenCalled();
  expect(
    screen.queryByRole("button", { name: translate("en", "tasks.openSource") }),
  ).not.toBeInTheDocument();
});

it.each([403, 404])(
  "describes an unavailable source without retry for %s",
  async (status) => {
    show(proposal, source, status);
    expect(
      await screen.findByText(translate("en", "email.detail.withheldReason")),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: translate("en", "common.retry") }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Review meeting/)).not.toBeInTheDocument();
  },
);

it("renders a date without a trailing separator when the source has no subject", async () => {
  show(proposal, { ...source, subject: "" });
  expect(
    await screen.findByText(formatDateTime(WHEN, "en", RECORD_ZONE)),
  ).toHaveTextContent(formatDateTime(WHEN, "en", RECORD_ZONE));
  expect(screen.queryByText(/ · $/)).not.toBeInTheDocument();
});

it("opens a lifecycle proposal's authorized original meeting once", async () => {
  show({
    ...proposal,
    kind: "lifecycle_change",
    target_entity_type: "company",
    target_entity_id: "company",
    preview: {
      version: 1,
      state: "ready",
      changes: [],
      reviewer_reason: "record_authority",
      evidence: [
        {
          source_type: "activity",
          source_id: SOURCE,
          evidence_snippet: "Contract ended",
        },
        {
          source_type: "activity",
          source_id: SOURCE,
          evidence_snippet: "No renewal",
        },
      ],
    },
  });
  expect(await screen.findByText(/Review meeting/)).toBeInTheDocument();
  const buttons = screen.getAllByRole("button", {
    name: translate("en", "tasks.openSource"),
  });
  expect(buttons).toHaveLength(1);
  await userEvent.setup().click(buttons[0]);
  expect(await screen.findByRole("dialog")).toHaveTextContent("Recorded words");
});

it("shows the authorized sender signature under the message", () => {
  const approval = messageApproval();
  if (!approval.preview) throw new Error("fixture needs preview");
  approval.preview.details = [
    ...(approval.preview.details ?? []),
    {
      field: "signature_text",
      value: { kind: "text", text: "Dana Seller — Account Director" },
    },
  ];
  show(approval);
  expect(
    screen.getByText("Dana Seller — Account Director"),
  ).toBeInTheDocument();
});

it("never renders an extension argument as message markup", () => {
  show({
    ...proposal,
    kind: "forget_record",
    preview: {
      version: 2,
      state: "ready",
      changes: [],
      reviewer_reason: "record_authority",
      details: ["html_body", "signature_html"].map((name) => ({
        field: `argument:${name}`,
        value: { kind: "text", text: "<b data-testid='injected'>bold</b>" },
      })),
    },
  });
  expect(screen.queryByTestId("injected")).not.toBeInTheDocument();
  expect(document.querySelector("iframe")).toBeNull();
});
