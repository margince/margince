// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { ApprovalRow } from "../features/automations/approvalrow";
import type { Locale } from "../i18n";
import type { GrantSpec } from "./mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type Approval = components["schemas"]["Approval"];

// `decide` is what the approve/reject buttons hit. A story that wants the row's
// refusal states answers that route with the problem document the server sends.
// `allow` and `routes` serve a card that reads more than the default grants and
// records.
export function ApprovalRowStory({
  subject,
  locale = "en",
  decided,
  decide,
  allow = { deal: ["read", "update"] },
  routes = {},
}: Readonly<{
  subject: Approval;
  locale?: Locale;
  decided?: boolean;
  decide?: () => Response;
  allow?: GrantSpec;
  routes?: RouteMap;
}>) {
  installFetchStub({
    ...routes,
    "GET /me": meRoute(allow),
    "GET /users/names": () =>
      jsonResponse({
        data: [
          { id: "original", display_name: "Theo" },
          { id: "assignee", display_name: "Mira" },
        ],
      }),
    "GET /activities/22222222-2222-4222-8222-222222222222": () =>
      jsonResponse({
        id: "22222222-2222-4222-8222-222222222222",
        kind: "meeting",
        subject: "Quarterly account review",
        body: "Alex: I will send the revised scope.",
        occurred_at: "2026-09-01T09:00:00Z",
        is_done: true,
        source: "manual",
        captured_by: "human:reviewer",
        created_at: "2026-09-01T09:00:00Z",
        updated_at: "2026-09-01T09:00:00Z",
        links: [],
        version: 1,
      }),
    "GET /agent-tools": () =>
      jsonResponse({
        data: [{ name: "send_email", tier: "confirm" }],
        page: { next_cursor: null, has_more: false },
      }),
    ...(decide
      ? {
          "POST /approvals/ap-1/approve": decide,
          "POST /approvals/ap-1/reject": decide,
        }
      : {}),
  });
  return (
    <StoryProviders locale={locale}>
      <ApprovalRow approval={subject} decided={decided} />
    </StoryProviders>
  );
}

export function approvalRowSubject(over: Partial<Approval>): Approval {
  return {
    id: "ap-1",
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary: "Send the follow-up to Anna Weber",
    proposed_change: {
      subject: "Following up on the depot quote",
      body: "Hallo Frau Weber, anbei die überarbeitete Kalkulation.",
    },
    confidence: 0.82,
    target_version: 3,
    on_behalf_of: "u-1",
    created_at: "2026-08-20T09:00:00Z",
    ...over,
  };
}
