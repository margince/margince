import type { components } from "../api/schema";

export const systemTask: components["schemas"]["Activity"] = {
  id: "11111111-1111-4111-8111-111111111111",
  kind: "task",
  is_done: false,
  subject: "Review qualification for Example",
  body: "Review the evidence and choose the next step.",
  occurred_at: "2026-11-09T08:00:00Z",
  occurred_precision: "instant",
  source: "automation",
  captured_by: "system:automation",
  created_at: "2026-11-09T08:00:00Z",
  updated_at: "2026-11-09T08:00:00Z",
  offers_email: false,
  version: 1,
  system_display: {
    locale: "vi",
    subject: "Xem xét điều kiện của Example",
    body: "Xem xét bằng chứng và chọn bước tiếp theo.",
    source_version: 1,
  },
};
