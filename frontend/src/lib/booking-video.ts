// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type VideoApp = NonNullable<
  components["schemas"]["PublicSchedulingProfile"]["video_app"]
>;
type Provider = NonNullable<
  components["schemas"]["MeetingInvitation"]["provider"]
>;

/** The app whose link a calendar provider adds to an event it holds. */
export const PROVIDER_VIDEO_APP: Readonly<Record<Provider, VideoApp>> = {
  gcal: "google_meet",
  graphcal: "microsoft_teams",
};

// Product names, which no locale translates, so they live here rather than
// as catalogue entries every language would have to repeat unchanged.
export const VIDEO_APP_NAME: Readonly<Record<VideoApp, string>> = {
  google_meet: "Google Meet",
  microsoft_teams: "Microsoft Teams",
};
