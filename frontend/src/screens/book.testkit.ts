import type { components } from "../api/schema";

export const bookingSlots = [
  { start: "2026-10-05T09:00:00Z", end: "2026-10-05T09:30:00Z" },
  { start: "2026-10-05T11:00:00Z", end: "2026-10-05T11:30:00Z" },
];
export const bookingProfile: components["schemas"]["SchedulingProfile"] = {
  provider: "gcal",
  calendar_id: "primary",
  enabled: true,
  duration_minutes: 30,
  notice_minutes: 120,
  buffer_minutes: 10,
  horizon_days: 30,
  title: "Let's talk about your next step",
  location: "Video call",
  host_name: "Ada Lovelace",
  company_name: "Gradion",
  slug: "ada-lovelace",
  public_url: "https://crm.example.test/#/book/ada-lovelace",
};
export const bookingInvitation: components["schemas"]["MeetingInvitation"] = {
  id: "0198f011-aaaa-7000-8000-000000000001",
  version: 1,
  status: "pending",
  ...bookingSlots[0],
  subject: "Project discovery",
  location: "Video call",
};
export const bookingContact = {
  id: "0198f011-aaaa-7000-8000-000000000002",
  full_name: "Nina Weber",
  primary_email: "nina@brandt.example",
};
