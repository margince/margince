// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";

// A mark this close to square fills a square chip without becoming a smear.
// Anything wider is a wordmark, which no one can read at that size.
const SQUARE_RATIO = 1.25;

export function isSquareMark(width: number, height: number): boolean {
  if (width <= 0 || height <= 0) return false;
  return Math.max(width, height) / Math.min(width, height) <= SQUARE_RATIO;
}

/**
 * The installation's square mark: the icon slot, else the wide slot once its
 * image proves square. The site read stores a square lockup in the wide slot
 * alone. Null until a mark is known, which leaves the monogram in the chip.
 */
export function useSquareMark(
  icon: string | null | undefined,
  wide: string | null | undefined,
): string | null {
  const [probe, setProbe] = useState<{ src: string; square: boolean } | null>(
    null,
  );
  useEffect(() => {
    if (icon || !wide) return;
    const image = new Image();
    let current = true;
    image.onload = () => {
      if (current) {
        setProbe({
          src: wide,
          square: isSquareMark(image.naturalWidth, image.naturalHeight),
        });
      }
    };
    image.onerror = () => {
      if (current) setProbe({ src: wide, square: false });
    };
    image.src = wide;
    return () => {
      current = false;
      image.onload = null;
      image.onerror = null;
    };
  }, [icon, wide]);
  if (icon) return icon;
  return wide && probe?.src === wide && probe.square ? wide : null;
}
