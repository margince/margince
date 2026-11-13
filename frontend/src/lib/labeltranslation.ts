// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type LabelTranslations = components["schemas"]["LabelTranslations"];

/**
 * An admin-named label as this reader sees it: the name in their UI language
 * when the admin gave one, else the stored label. The stored label stays the
 * value a record holds; only its display changes.
 */
export function translatedLabel(
  label: string,
  translations: LabelTranslations | undefined,
  locale: string,
): string {
  const named = translations?.[locale as keyof LabelTranslations];
  return named?.trim() ? named : label;
}

type Stage = components["schemas"]["Stage"];
type Pipeline = components["schemas"]["Pipeline"];

/** One pipeline with every stage named in the reader's language. */
export function translatedPipeline(
  pipeline: Pipeline,
  locale: string,
): Pipeline {
  return {
    ...pipeline,
    stages: pipeline.stages?.map(
      (stage: Stage): Stage => ({
        ...stage,
        name: translatedLabel(stage.name, stage.name_translations, locale),
      }),
    ),
  };
}
