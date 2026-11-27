// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export const COMPANY_LEADS_VIEW: components["schemas"]["Company360"] = {
  as_of: "2026-09-01T09:00:00Z",
  company: {
    id: "company-1",
    display_name: "Brandt Automotive GmbH",
    source: "manual",
    captured_by: "human:rep-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
  },
  sections_omitted: [],
  leads: {
    data: [
      {
        id: "lead-1",
        full_name: "Dana Buyer",
        email: "dana@buyer.example",
        company_id: "company-1",
        status: "new",
        score: 40,
        source: "manual",
        captured_by: "human:rep-1",
        version: 1,
        created_at: "2026-09-01T08:00:00Z",
        updated_at: "2026-09-01T08:00:00Z",
      },
    ],
    page: { has_more: false, next_cursor: null },
  },
};
