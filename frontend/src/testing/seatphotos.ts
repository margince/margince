// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Stand-ins for a seat's picture, because a story serves no object store.

const svg = (body: string) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 96 96">${body}</svg>`,
  )}`;

/** A head and shoulders on a coloured ground. */
export const PORTRAIT = svg(
  '<rect width="96" height="96" fill="steelblue"/><circle cx="48" cy="38" r="18" fill="peachpuff"/><path d="M14 96c4-22 18-32 34-32s30 10 34 32z" fill="darkslategray"/>',
);

/** White to its edge: on a white ground only the ring shows where it ends. */
export const WHITE_PORTRAIT = svg(
  '<rect width="96" height="96" fill="white"/><circle cx="48" cy="40" r="16" fill="whitesmoke"/>',
);
