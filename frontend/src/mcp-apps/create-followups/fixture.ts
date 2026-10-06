import type { Envelope } from "../types";

/**
 * One create_record answer that filed a duplicate, shaped as the tool seals it.
 *
 * MIRRORED BY HAND from agents.createdRecord and agents.DuplicateCandidate.
 * Left is the record this create wrote, Right the one already on file.
 */
export const createFollowupsFixture: Envelope = {
  data: {
    record_type: "contact",
    id: "0195c3a0-0000-7000-8000-000000000002",
    fields: { full_name: "Anna Meyer" },
    version: 1,
    duplicate_candidates: [
      {
        candidate_id: "0195c3a0-0000-7000-8000-0000000000aa",
        other_record_id: "0195c3a0-0000-7000-8000-000000000001",
        confidence: 0.92,
        evidence: [
          {
            field: "full_name",
            left_value: "Anna Meyer",
            right_value: "Anna Meyer",
            signal: "collide",
            score: 0.98,
          },
          {
            field: "phone",
            left_value: "+49 30 1234567",
            right_value: "+49301234567",
            signal: "collide",
            score: 0.9,
          },
        ],
      },
    ],
  },
  warnings: [{ code: "duplicate_filed_for_review" }],
};
