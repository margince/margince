// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A declared mirror of AvatarMediaTypes and AvatarFileMaxBytes in
// backend/internal/compose/avatar.go. TestTheFrontendPictureLimitsMatchTheServer
// (backend/gates/frontendpicturelimits_test.go) fails when the two drift either way.

// The picture types an upload may be.
export const PICTURE_TYPES: readonly string[] = [
  "image/png",
  "image/jpeg",
  "image/webp",
];

// The largest file whose upload, with the form's own framing, fits the route's body ceiling.
export const PICTURE_MAX_BYTES = 4_983_616;
