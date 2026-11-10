// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { MAX_ACTIVITY_LINKS } from "../../lib/activitybody";
import { composedLinks } from "./composelinks";

const contactIds = Array.from({ length: 30 }, (_, i) => `per-${i + 1}`);

describe("the links an account send carries", () => {
  it("stays within the server's cap, the page, deal and project first", () => {
    const links = composedLinks(
      { entityType: "company", entityId: "company-1" },
      { contactIds, dealId: "deal-1", projectId: "proj-1" },
    );

    expect(links).toHaveLength(MAX_ACTIVITY_LINKS);
    expect(links.filter((link) => link.entity_type !== "contact")).toEqual([
      { entity_type: "company", entity_id: "company-1" },
      { entity_type: "deal", entity_id: "deal-1" },
      { entity_type: "project", entity_id: "proj-1" },
    ]);
    expect(
      links
        .filter((link) => link.entity_type === "contact")
        .map((link) => link.entity_id),
    ).toEqual(contactIds.slice(0, MAX_ACTIVITY_LINKS - 3));
  });

  it("spends no room on a contact named twice", () => {
    const links = composedLinks(
      { entityType: "company", entityId: "company-1" },
      {
        contactIds: ["per-1", "per-1", "per-2"],
        dealId: "",
        projectId: "",
      },
    );

    expect(links).toEqual([
      { entity_type: "company", entity_id: "company-1" },
      { entity_type: "contact", entity_id: "per-1" },
      { entity_type: "contact", entity_id: "per-2" },
    ]);
  });
});
