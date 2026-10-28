import type { components } from "../../api/schema";

export const recordedContact: components["schemas"]["Contact360"] = {
  as_of: "2026-08-24T12:00:00Z",
  contact: {
    id: "demo-contact",
    full_name: "Dana Buyer",
    visibility: "workspace",
    source: "manual",
    captured_by: "human:demo-user",
    created_at: "2026-01-01T12:00:00Z",
    updated_at: "2026-08-24T12:00:00Z",
  },
  sections_omitted: [],
  last_inbound_at: "2026-08-23T12:00:00Z",
  last_outbound_at: "2026-02-05T12:00:00Z",
};
