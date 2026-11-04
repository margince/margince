// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";

type Product = components["schemas"]["Product"];
type Translation = components["schemas"]["ProductTranslation"];
type Draft = Readonly<{ language: string; name: string; description: string }>;

const EMPTY: Draft = { language: "", name: "", description: "" };
const translationsKey = (id: string) => ["product", id, "translations"];

// A product's name and description in further languages. An offer in one of
// them takes the product's text from here when a line is added. A line already
// on an offer keeps the text it was added with.
export function ProductTranslationsDialog({
  product,
  onClose,
}: Readonly<{ product: Product; onClose: () => void }>) {
  const t = useT();
  const formId = useId();
  const client = useQueryClient();
  const list = useQuery({
    queryKey: translationsKey(product.id),
    queryFn: async () =>
      unwrap(
        await api.GET("/products/{id}/translations", {
          params: { path: { id: product.id } },
        }),
      ).data,
  });
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const refresh = () =>
    client.invalidateQueries({ queryKey: translationsKey(product.id) });
  const save = useMutation({
    mutationFn: async (input: Draft) =>
      unwrap(
        await api.PUT("/products/{id}/translations/{language}", {
          params: { path: { id: product.id, language: input.language } },
          body: {
            name: input.name.trim(),
            description: input.description.trim() || undefined,
          },
        }),
      ),
    onSuccess: () => {
      setDraft(EMPTY);
      void refresh();
    },
  });
  const remove = useMutation({
    mutationFn: async (language: string) =>
      unwrap(
        await api.DELETE("/products/{id}/translations/{language}", {
          params: { path: { id: product.id, language } },
        }),
      ),
    onSuccess: () => void refresh(),
  });
  const set = (key: keyof Draft) => (value: string) =>
    setDraft((current) => ({ ...current, [key]: value }));
  const submit = (event: FormEvent) => {
    event.preventDefault();
    save.mutate({ ...draft, language: draft.language.trim().toLowerCase() });
  };
  const translations: readonly Translation[] = list.data ?? [];
  return (
    <RecordFormDialog
      open
      title={t("product.translationsTitle", { name: product.name })}
      onClose={onClose}
      fieldCount={3}
      unsaved={draft.name.trim() !== "" || draft.language.trim() !== ""}
      closeDisabled={save.isPending}
      form={
        <form id={formId} onSubmit={submit} className="form-stack">
          <p className="t-caption">{t("product.translationsHint")}</p>
          {translations.length === 0 ? (
            <EmptyState>{t("product.translationsEmpty")}</EmptyState>
          ) : (
            <ul className="form-stack">
              {translations.map((translation) => (
                <li key={translation.language}>
                  <span className="t-name">
                    {translation.language} · {translation.name}
                  </span>{" "}
                  <Button
                    variant="ghost"
                    pending={
                      remove.isPending &&
                      remove.variables === translation.language
                    }
                    onClick={() => remove.mutate(translation.language)}
                  >
                    {t("product.translationRemove", {
                      language: translation.language,
                    })}
                  </Button>
                  {translation.description && (
                    <p className="t-caption">{translation.description}</p>
                  )}
                </li>
              ))}
            </ul>
          )}
          <Field
            label={t("product.translationLanguage")}
            hint={t("product.translationLanguageHint")}
            required
          >
            {(control) => (
              <TextInput
                {...control}
                required
                maxLength={2}
                value={draft.language}
                onChange={(event) => set("language")(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("product.name")} required>
            {(control) => (
              <TextInput
                {...control}
                required
                maxLength={500}
                value={draft.name}
                onChange={(event) => set("name")(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("product.description")}>
            {(control) => (
              <Textarea
                {...control}
                rows={3}
                maxLength={4000}
                value={draft.description}
                onChange={(event) => set("description")(event.target.value)}
              />
            )}
          </Field>
          <ErrorLine
            error={
              (save.isError && problemMessageOf(save.error, t)) ||
              (remove.isError && problemMessageOf(remove.error, t))
            }
          />
        </form>
      }
      actions={
        <>
          <Button disabled={save.isPending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            variant="primary"
            pending={save.isPending}
          >
            {t("product.translationSave")}
          </Button>
        </>
      }
    />
  );
}
