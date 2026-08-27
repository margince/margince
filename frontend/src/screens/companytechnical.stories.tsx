// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { TechnicalProfileCard } from "./companytechnical";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a company publicly runs, on its record.
//
// The states worth looking at are not the full card. They are the ones that
// say something a reader has to act on: an account nobody has looked up yet,
// an account where one SOURCE did not answer (so part of the card is older
// than the rest), and a reader who may not start a lookup at all.

const meta: Meta = {
  title: "Records/Company 360/Technical profile",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ORG = "019ff000-0000-7000-8000-0000000000a1";

/** A technical fact as the contract sends it: the value, and the public record
 * that proved it. */
function fact(
  field: string,
  valueKey: string,
  value: string,
  evidence: string,
  sourceURL: string,
) {
  return {
    id: `fact-${field}-${valueKey}`,
    category: "signal",
    field,
    value,
    value_key: valueKey,
    source: "technical_lookup",
    captured_by: "agent:technical-lookup",
    evidence_snippet: evidence,
    source_url: sourceURL,
    confidence: 0.9,
    retrieved_at: "2026-08-27T07:12:00Z",
    updated_at: "2026-08-27T07:12:00Z",
  };
}

const READ_FACTS = [
  fact(
    "mail_provider",
    "microsoft365",
    "Microsoft 365",
    "beispiel-de.mail.protection.outlook.com",
    "dns:beispiel.de",
  ),
  fact(
    "email_security",
    "dmarc_reject",
    "DMARC durchgesetzt",
    "v=DMARC1; p=reject; rua=mailto:dmarc@beispiel.de",
    "dns:beispiel.de",
  ),
  fact(
    "technology",
    "shopware",
    "Shopware",
    "meta generator: Shopware 6",
    "https://beispiel.de",
  ),
  fact(
    "technology",
    "matomo",
    "Matomo",
    "Script: https://beispiel.de/matomo.js",
    "https://beispiel.de",
  ),
  fact(
    "operated_service",
    "webshop",
    "Webshop",
    "shop.beispiel.de",
    "https://crt.sh/?q=%25.beispiel.de",
  ),
  fact(
    "operated_service",
    "careers",
    "Karriereseite",
    "karriere.beispiel.de",
    "https://crt.sh/?q=%25.beispiel.de",
  ),
  fact(
    "hosting_provider",
    "hetzner",
    "Hetzner",
    "static.88.99.beispiel.your-server.de",
    "dns:beispiel.de",
  ),
];

function lanes(
  overrides: Record<string, string> = {},
): Record<string, unknown> {
  return {
    organization_id: ORG,
    lanes: ["dns", "certlog", "homepage"].map((lane) => ({
      lane,
      outcome: overrides[lane] ?? "applied",
      attempts: overrides[lane] === "failed" ? 3 : 0,
      last_success_at:
        overrides[lane] === "failed"
          ? "2026-08-20T07:00:00Z"
          : "2026-08-27T07:12:00Z",
      next_attempt_at: "2026-09-03T07:12:00Z",
    })),
  };
}

const CAN_ENRICH = meRoute({ organization: ["read", "update"] });

/** Everything read: the state a rep sees on an account the lookup has covered. */
export const Read: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({ data: READ_FACTS }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(lanes()),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};

/**
 * Never looked up. The card offers the lookup and says plainly that there is
 * nothing yet — an empty card with no sentence reads as "this company runs
 * nothing", which is a different and false claim.
 */
export const NeverLookedUp: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () => jsonResponse({ data: [] }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse({ title: "not found" }, 404),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};

/**
 * The certificate log did not answer.
 *
 * The services it reads are absent rather than wrong — what it saw last week
 * still stands — and the notice at the foot is what tells a reader that "no
 * webshop" here means "not checked today" rather than "they have none".
 */
export const OneSourceDidNotAnswer: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({
          data: READ_FACTS.filter((row) => row.field !== "operated_service"),
        }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(lanes({ certlog: "failed" })),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};

/** The site's robots.txt declined the homepage read — an ANSWER, not a
 * failure, so the technologies are genuinely absent rather than stale. */
export const TheSiteDeclined: Story = {
  render: () => {
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({
          data: READ_FACTS.filter((row) => row.field !== "technology"),
        }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(lanes({ homepage: "refused" })),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};

/** A reader who may not write the record sees the profile and no button: the
 * lookup writes to the company, so it is gated like any other write. */
export const WithoutWriteAccess: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ organization: ["read"] }, { seat: "read" }),
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({ data: READ_FACTS }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(lanes()),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};

/** A human corrected a machine-read value. The row stays on this card — it is
 * still a technical field — and loses its evidence mark, because a value a
 * person typed is not a claim the product is making. */
export const AfterAHumanCorrection: Story = {
  render: () => {
    const corrected = READ_FACTS.map((row) =>
      row.field === "mail_provider"
        ? {
            ...row,
            value: "Eigener Mailserver",
            source: "human",
            captured_by: "human:019ff000-0000-7000-8000-0000000000b2",
          }
        : row,
    );
    installFetchStub({
      "GET /me": CAN_ENRICH,
      [`GET /organizations/${ORG}/facts`]: () =>
        jsonResponse({ data: corrected }),
      [`GET /organizations/${ORG}/technical-enrich/latest`]: () =>
        jsonResponse(lanes()),
    });
    return (
      <StoryProviders locale="de">
        <TechnicalProfileCard orgId={ORG} />
      </StoryProviders>
    );
  },
};
