// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

export type DeviceLabel = Readonly<{ browser?: string; system?: string }>;

// Order matters: Edge and Opera send "Chrome", and Chrome sends "Safari".
const BROWSERS: readonly (readonly [RegExp, string])[] = [
  [/Edg(e|A|iOS)?\//, "Edge"],
  [/OPR\/|Opera/, "Opera"],
  [/Firefox\/|FxiOS\//, "Firefox"],
  [/Chrome\/|CriOS\//, "Chrome"],
  [/Safari\//, "Safari"],
];

// iPhone and Android agents also say "Mac OS X" or "Linux", so they come first.
const SYSTEMS: readonly (readonly [RegExp, string])[] = [
  [/iPhone|iPad|iPod/, "iOS"],
  [/Android/, "Android"],
  [/Windows/, "Windows"],
  [/Macintosh|Mac OS X/, "macOS"],
  [/CrOS/, "ChromeOS"],
  [/Linux|X11/, "Linux"],
];

function firstMatch(
  agent: string,
  table: readonly (readonly [RegExp, string])[],
): string | undefined {
  return table.find(([pattern]) => pattern.test(agent))?.[1];
}

/** The browser and system a User-Agent names, each absent when it names none. */
export function deviceLabel(userAgent: string | null | undefined): DeviceLabel {
  if (!userAgent) return {};
  return {
    browser: firstMatch(userAgent, BROWSERS),
    system: firstMatch(userAgent, SYSTEMS),
  };
}
