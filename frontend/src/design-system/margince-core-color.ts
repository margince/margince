// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Shader uniforms read the same literal token colors as the static decoration.
export function readHue(name: string): readonly [number, number, number] {
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  const hex = /^#([0-9a-f]{6})$/i.exec(raw)?.[1];
  if (hex === undefined) {
    return [0.5, 0.5, 0.5];
  }
  const value = Number.parseInt(hex, 16);
  return [
    ((value >> 16) & 255) / 255,
    ((value >> 8) & 255) / 255,
    (value & 255) / 255,
  ];
}
