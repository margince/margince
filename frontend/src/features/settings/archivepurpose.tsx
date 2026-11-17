// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Field, Textarea } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { languageName } from "../../format/languagename";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemFieldSentencesOf, throwProblem } from "../../lib/problem";
import { languagesOf, PARTICULARS_QUERY_KEY } from "./controllerparticulars";

// Settings → Privacy → Retention: why the archive keeps correspondence, as the
// privacy page states it. The words are part of the controller particulars, so
// a save sends the stored particulars back with only these words changed.

type Particulars = components["schemas"]["ControllerParticulars"];

type Draft = Readonly<{
  purpose: string;
  translations: Readonly<Record<string, string>>;
}>;

function draftOf(particulars: Particulars): Draft {
  const stated = new Map(
    (particulars.translations ?? []).map((t) => [t.language, t]),
  );
  return {
    purpose: particulars.archive_purpose ?? "",
    translations: Object.fromEntries(
      languagesOf(particulars).map((language) => [
        language,
        stated.get(language)?.archive_purpose ?? "",
      ]),
    ),
  };
}

/** The stored particulars with the purpose words replaced. A language with no
 * translation yet gains one that states the purpose alone. */
export function archivePurposeBody(
  particulars: Particulars,
  draft: Draft,
): Particulars {
  const {
    locks: _locks,
    disclosure_languages: _languages,
    objection_route_means: _means,
    ...stored
  } = particulars;
  const kept = (stored.translations ?? []).map((t) => ({
    ...t,
    archive_purpose: draft.translations[t.language] ?? t.archive_purpose,
  }));
  const added = Object.entries(draft.translations)
    .filter(
      ([language, purpose]) =>
        purpose.trim() !== "" && !kept.some((t) => t.language === language),
    )
    .map(([language, purpose]) => ({ language, archive_purpose: purpose }));
  return {
    ...stored,
    archive_purpose: draft.purpose,
    translations: [...kept, ...added],
  };
}

export function ArchivePurposeCard() {
  const t = useT();
  const query = useQuery({
    queryKey: PARTICULARS_QUERY_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/privacy/controller-particulars",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("archivePurpose.title")}>
      {(particulars) => <ArchivePurposeEditor particulars={particulars} />}
    </QueryGate>
  );
}

function ArchivePurposeEditor({
  particulars,
}: Readonly<{ particulars: Particulars }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const canEdit = useCanWrite("installation_settings", "update");
  const saved = draftOf(particulars);
  const [draft, setDraft] = useState<Draft | null>(null);
  const value = draft ?? saved;
  const save = useMutation({
    mutationFn: async (body: Particulars) => {
      const { data, error, response } = await api.PUT(
        "/privacy/controller-particulars",
        { body },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => client.setQueryData(PARTICULARS_QUERY_KEY, data),
  });
  const languages = Object.keys(value.translations);
  useSaveDraft({
    dirty: JSON.stringify(value) !== JSON.stringify(saved),
    save: async () => {
      await save.mutateAsync(archivePurposeBody(particulars, value));
      setDraft(null);
    },
    placed: [
      "archive_purpose",
      ...languages.map(
        (language) => `translations.${language}.archive_purpose`,
      ),
    ],
    discard: () => setDraft(null),
  });
  const faults = problemFieldSentencesOf(save.error, locale);
  const hint = t("archivePurpose.hint", {
    purpose: t("privacynotice.retention.archivePurpose"),
  });
  const box = (
    label: string,
    field: string,
    text: string,
    onChange: (next: string) => void,
  ) => (
    <Field key={field} label={label} hint={hint} error={faults.get(field)}>
      {(control) => (
        <Textarea
          {...control}
          rows={2}
          maxLength={500}
          disabled={!canEdit || save.isPending}
          value={text}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );

  return (
    <Panel title={t("archivePurpose.title")}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("archivePurpose.intro")}</PanelIntro>
        {box(
          t("archivePurpose.label"),
          "archive_purpose",
          value.purpose,
          (next) => setDraft({ ...value, purpose: next }),
        )}
        {languages.map((language) =>
          box(
            t("archivePurpose.translationLabel", {
              language: languageName(language, locale),
            }),
            `translations.${language}.archive_purpose`,
            value.translations[language] ?? "",
            (next) =>
              setDraft({
                ...value,
                translations: { ...value.translations, [language]: next },
              }),
          ),
        )}
      </PanelBody>
    </Panel>
  );
}
