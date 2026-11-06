// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  alternativesOf,
  appStylesheets,
  classesOf,
  compounds,
  functionCall,
  type Rule,
  rulesOf,
  simpleSelectorsOf,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { ladderLists } from "../../scripts/lib/surface-ladder";

// What the surface gates read: the app's sheets, the ladder and the parsing
// both surfaces.test.ts and surfaces.layers.test.ts apply to them, kept once.

export const here = dirname(fileURLToPath(import.meta.url));
export const frontendRoot = join(here, "..", "..");
export const ladderPath = join(here, "surfaces.css");
export const ladderText = readFileSync(ladderPath, "utf8");

export const VAR = /var\((--[\w-]+)\)/g;
export const ALIAS = /^var\((--[\w-]+)\)$/;
export const OPAQUE = /^#[0-9a-f]{6}$/i;
export const TYPE = /^(?:[a-zA-Z][\w-]*|\*)/;
export const PSEUDO_ELEMENT = /::[\w-]+|:(?:before|after)\b/;
export const STATE = /:(?:hover|focus|focus-visible|focus-within|active)\b/g;

export type Declaration = { property: string; value: string };

export function declarations(body: string): Declaration[] {
  return splitTopLevel(body, ";").flatMap((part) => {
    const colon = part.indexOf(":");
    if (colon < 0) return [];
    return [
      {
        property: part.slice(0, colon).trim(),
        value: part
          .slice(colon + 1)
          .trim()
          .replace(/\s+/g, " "),
      },
    ];
  });
}

export const sheets = appStylesheets(frontendRoot);
export const allRules = rulesOf(sheets);

export const lists = ladderLists(ladderText);

export const probe = (selector: string, body: string): Rule => ({
  file: join(here, "probe.css"),
  selector,
  body,
});

export type Written = {
  own: ReadonlySet<string>;
  context: ReadonlySet<string>;
  type: string;
  ownSimples: ReadonlySet<string>;
  contextSimples: ReadonlySet<string>;
};

export function written(selector: string): Written {
  const parts = compounds(selector);
  const subject = parts.at(-1) ?? "";
  const around = parts.slice(0, -1);
  return {
    own: classesOf(subject),
    context: new Set(around.flatMap((part) => [...classesOf(part)])),
    type: TYPE.exec(subject)?.[0] ?? "",
    ownSimples: simpleSelectorsOf(subject),
    contextSimples: new Set(
      around.flatMap((part) => [...simpleSelectorsOf(part)]),
    ),
  };
}

export const membersOf = (selectors: readonly string[]): Written[] =>
  selectors.flatMap((entry) => alternativesOf(entry).map(written));

export const within = (
  part: ReadonlySet<string>,
  ...wholes: readonly ReadonlySet<string>[]
) => [...part].every((name) => wholes.some((whole) => whole.has(name)));

export function flatFill(layer: string): string | undefined {
  const gradient = functionCall(layer);
  if (gradient?.name !== "linear-gradient") return undefined;
  const stops = gradient.args.map((stop) => stop.replace(/\s+[\d.]+%$/, ""));
  return stops.every((stop) => stop === stops[0]) ? stops[0] : undefined;
}

export const IMAGE = /^(?:[\w-]+-gradient|url)\(/;

export const MEMBERS = membersOf([...lists.surfaces, ...lists.overlays]);
