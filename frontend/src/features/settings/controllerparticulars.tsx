// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Field, SectionHeader, Textarea } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { languageName } from "../../format/languagename";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { problemFieldSentencesOf, throwProblem } from "../../lib/problem";

// Settings → Governance → Jurisdiction: what every mail says about the
// installation, and the same text in each language a pack in force asks for.

type Particulars = components["schemas"]["ControllerParticulars"];
type Translation = components["schemas"]["ControllerParticularsTranslation"];

const QUERY_KEY = ["controller-particulars"] as const;

const FIELDS = [
  "legal_name",
  "postal_address",
  "privacy_contact",
  "objection_route",
  "advertiser_contact",
] as const;
type FieldKey = (typeof FIELDS)[number];
type Text = Record<FieldKey, string>;

const LABEL: Record<FieldKey, MessageKey> = {
  legal_name: "particulars.legalName",
  postal_address: "particulars.postalAddress",
  privacy_contact: "particulars.privacyContact",
  objection_route: "particulars.objectionRoute",
  advertiser_contact: "particulars.advertiserContact",
};

// A route that meets a pack's three means, written in that pack's language. It
// is content to copy, so it reads the same in every interface language.
const OBJECTION_EXAMPLE: Readonly<Record<string, string>> = {
  vi: "trả lời email này với từ STOP, gửi email đến privacy@example.com, gọi số +84 00 0000 0000 hoặc dùng trang [đường dẫn trang tùy chọn nhận tin].",
};

type Draft = Readonly<{
  text: Text;
  translations: Readonly<Record<string, Text>>;
}>;

function textOf(source: Partial<Record<FieldKey, string | null>>): Text {
  return {
    legal_name: source.legal_name ?? "",
    postal_address: source.postal_address ?? "",
    privacy_contact: source.privacy_contact ?? "",
    objection_route: source.objection_route ?? "",
    advertiser_contact: source.advertiser_contact ?? "",
  };
}

// The languages the form offers: those a pack in force asks for, then any
// already stated.
function languagesOf(particulars: Particulars): string[] {
  const stated = (particulars.translations ?? []).map((t) => t.language);
  return [...new Set([...(particulars.disclosure_languages ?? []), ...stated])];
}

function draftOf(particulars: Particulars): Draft {
  const stated = new Map(
    (particulars.translations ?? []).map((t) => [t.language, t]),
  );
  return {
    text: textOf(particulars),
    translations: Object.fromEntries(
      languagesOf(particulars).map((language) => [
        language,
        textOf(stated.get(language) ?? { language }),
      ]),
    ),
  };
}

// A translation with nothing in it is not sent, so an empty section saves no
// row.
function bodyOf(draft: Draft): Particulars {
  const translations: Translation[] = Object.entries(draft.translations)
    .filter(([, text]) => FIELDS.some((key) => text[key].trim() !== ""))
    .map(([language, text]) => ({ language, ...text }));
  return { ...draft.text, translations };
}

export function ControllerParticularsCard() {
  const t = useT();
  const query = useQuery({
    queryKey: QUERY_KEY,
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
    <QueryGate query={query} pendingLabel={t("particulars.title")}>
      {(particulars) => <ParticularsEditor particulars={particulars} />}
    </QueryGate>
  );
}

function ParticularsEditor({
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
    onSuccess: (data) => client.setQueryData(QUERY_KEY, data),
  });
  const languages = Object.keys(value.translations);
  useSaveDraft({
    dirty: JSON.stringify(value) !== JSON.stringify(saved),
    save: async () => {
      await save.mutateAsync(bodyOf(value));
      setDraft(null);
    },
    placed: [
      ...FIELDS,
      ...languages.flatMap((language) =>
        FIELDS.map((key) => `translations.${language}.${key}`),
      ),
    ],
    discard: () => setDraft(null),
  });
  const faults = problemFieldSentencesOf(save.error, locale);
  const means = particulars.objection_route_means ?? [];
  const meansHint =
    means.length > 0 ? t("particulars.objectionMeans") : undefined;

  const fields = (
    text: Text,
    prefix: string,
    hintFor: (key: FieldKey) => string | undefined,
    onChange: (key: FieldKey, next: string) => void,
  ) =>
    FIELDS.map((key) => (
      <Field
        key={key}
        label={t(LABEL[key])}
        hint={hintFor(key)}
        error={faults.get(`${prefix}${key}`)}
      >
        {(control) => (
          <Textarea
            {...control}
            rows={2}
            maxLength={500}
            disabled={!canEdit || save.isPending}
            value={text[key]}
            onChange={(event) => onChange(key, event.target.value)}
          />
        )}
      </Field>
    ));

  return (
    <Panel title={t("particulars.title")}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("particulars.intro")}</PanelIntro>
        {fields(
          value.text,
          "",
          (key) => (key === "objection_route" ? meansHint : undefined),
          (key, next) =>
            setDraft({ ...value, text: { ...value.text, [key]: next } }),
        )}
        {languages.map((language) => {
          const example = OBJECTION_EXAMPLE[language];
          return (
            <section
              key={language}
              className="form-stack"
              aria-label={languageName(language, locale)}
            >
              <SectionHeader
                level={3}
                title={t("particulars.translationTitle", {
                  language: languageName(language, locale),
                })}
              />
              <p className="t-caption">
                {t("particulars.translationIntro", {
                  language: languageName(language, locale),
                })}
              </p>
              {fields(
                value.translations[language] ?? textOf({}),
                `translations.${language}.`,
                (key) =>
                  key === "objection_route" && example
                    ? t("particulars.objectionExample", { example })
                    : undefined,
                (key, next) =>
                  setDraft({
                    ...value,
                    translations: {
                      ...value.translations,
                      [language]: {
                        ...(value.translations[language] ?? textOf({})),
                        [key]: next,
                      },
                    },
                  }),
              )}
            </section>
          );
        })}
      </PanelBody>
    </Panel>
  );
}
