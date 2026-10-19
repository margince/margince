// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ImportFormat, ImportRun } from "./importtypes";

// Wire answers the import screens' stories and tests share.

// Three files in the server's order. Contacts link to companies by
// `company_ref`, and companies carry one custom picklist. So the guide, the
// template and the map all have a link and a custom column to show.
export const formatFixture: ImportFormat = {
  version: "margince-import/1",
  order: ["company", "contact", "lead"],
  files: [
    {
      file: "companies.csv",
      object: "company",
      columns: [
        {
          name: "ref",
          description: "Your own key for this row.",
          example: "CO-0042",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "display_name",
          description: "The company’s name.",
          example: "Acme GmbH",
          required_on_new_row: true,
          custom: false,
        },
        {
          name: "domain",
          description: "The company’s web domain.",
          example: "acme.com",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "cf_tier",
          label: "Tier",
          description: "A custom picklist.",
          example: "A",
          required_on_new_row: false,
          custom: true,
          type: "picklist",
          options: ["A", "B"],
        },
      ],
    },
    {
      file: "contacts.csv",
      object: "contact",
      columns: [
        {
          name: "ref",
          description: "Your own key for this row.",
          example: "P-17",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "email",
          description: "The contact’s email address.",
          example: "jo@acme.com",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "company_ref",
          description: "Which company this row belongs to.",
          example: "CO-0042",
          required_on_new_row: false,
          custom: false,
        },
      ],
    },
    {
      file: "leads.csv",
      object: "lead",
      columns: [
        {
          name: "full_name",
          description: "The prospect’s whole name.",
          example: "Jo Tran",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "email",
          description: "The prospect’s email address.",
          example: "jo@acme.com",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "title",
          description: "The prospect’s job title.",
          example: "Head of Purchasing",
          required_on_new_row: false,
          custom: false,
        },
        {
          name: "company_name",
          description: "The company the prospect names, as text.",
          example: "Acme",
          required_on_new_row: false,
          custom: false,
        },
      ],
    },
  ],
  key_rules: ["A row with the same ref updates the record it made before."],
  size_limit_mb: 10,
};

// Two runs as the list answers them, newest first: one complete ZIP that may
// still be undone, and one CSV that already was.
export const recentRunsFixture: ImportRun[] = [
  {
    id: "019ff-run-zip",
    connector: "csv",
    file_name: "hubspot-export.zip",
    source_name: "hubspot-export",
    status: "complete",
    checkpoint: 1240,
    version: 2,
    source_released: true,
    source: "import_api",
    captured_by: "u-ada",
    parts: [
      { file: "companies.csv", object: "company", row_count: 140 },
      { file: "contacts.csv", object: "contact", row_count: 1020 },
      { file: "leads.csv", object: "lead", row_count: 80 },
    ],
    created_at: "2026-10-12T09:30:00Z",
    updated_at: "2026-10-12T09:32:00Z",
  },
  {
    id: "019ff-run-csv",
    connector: "csv",
    object: "lead",
    file_name: "leads-webinar.csv",
    status: "undone",
    checkpoint: 0,
    version: 2,
    source_released: true,
    source: "import_api",
    captured_by: "u-ada",
    parts: [{ file: "leads-webinar.csv", object: "lead", row_count: 80 }],
    created_at: "2026-10-02T14:00:00Z",
    updated_at: "2026-10-02T15:00:00Z",
  },
];
