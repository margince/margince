import type { Envelope } from "../types";

/**
 * One update_record answer, shaped as the tool seals it: the record as it
 * stands after the write, and what each field was and is now.
 *
 * MIRRORED BY HAND from agents.UpdateResult.
 */
export const updateFixture: Envelope = {
  data: {
    record_type: "contact",
    id: "0195c3a0-0000-7000-8000-000000000003",
    fields: { full_name: "Anna Meyer", job_title: "VP Sales" },
    version: 5,
    changes: [
      {
        field: "job_title",
        label: "Job title",
        before: { kind: "text", text: "Head of Sales" },
        after: { kind: "text", text: "VP Sales" },
      },
    ],
  },
  warnings: [],
};
