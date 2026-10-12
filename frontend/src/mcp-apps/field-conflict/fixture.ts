import type { Envelope } from "../types";

/**
 * One update_record answer where some fields applied and one was held for an
 * answer, shaped as the tool seals it.
 *
 * MIRRORED BY HAND from agents.UpdateWithStagedApprovalResult. The record's own
 * fields are the CURRENT values: the held field was not written.
 */
export const fieldConflictFixture: Envelope = {
  data: {
    record_type: "contact",
    id: "0195c3a0-0000-7000-8000-000000000003",
    fields: { full_name: "Anna Meyer", job_title: "Head of Sales" },
    version: 4,
    staged_approval: {
      approval_id: "0195c3a0-0000-7000-8000-0000000000c1",
      fields: ["job_title"],
      replay: {
        record_type: "contact",
        id: "0195c3a0-0000-7000-8000-000000000003",
        fields: { job_title: "VP Sales" },
      },
      message: "job_title was last edited by hand, so the change is waiting.",
    },
  },
  warnings: [],
};
