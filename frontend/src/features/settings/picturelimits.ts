// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A declared mirror of AvatarMediaTypes, AvatarFileMaxBytes and AvatarMaxPixels in
// backend/internal/compose/avatar.go. TestTheFrontendPictureLimitsMatchTheServer
// (backend/gates/frontendpicturelimits_test.go) fails when the two drift either way.

// The picture types an upload may be.
export const PICTURE_TYPES: readonly string[] = [
  "image/png",
  "image/jpeg",
  "image/webp",
];

// The largest picture file an upload may carry. The route adds the form's framing to it.
export const PICTURE_MAX_BYTES = 5_000_000;

// The most pixels the server decodes. The row shrinks a larger picture before it sends it.
export const PICTURE_MAX_PIXELS = 8_388_608;
