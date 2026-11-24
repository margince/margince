// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { Button } from "../design-system/button";
import { Callout } from "../design-system/callout";
import { Field } from "../design-system/field";
import { Select } from "../design-system/select";
import { countryOptions } from "../format/assignedcountries";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import { SEND_PERMISSION_KEY } from "../lib/usesendpermission";

// The question the preview asks when an unknown country could turn a refusal
// into an allow. The answer is saved through the ordinary contact edit, so it
// is audited, and the preview is asked again.

type Preview = components["schemas"]["SendAuthorizationPreview"];

type Asked = Readonly<{ address: string; contactId: string }>;

/** The contacts the preview asks a country for. A lead is never asked. */
export function countriesAskedFor(preview: Preview | undefined): Asked[] {
  return (preview?.recipients ?? []).flatMap((recipient) => {
    const subject = recipient.evidence_subject;
    return recipient.ask_country === true && subject?.entity_type === "contact"
      ? [{ address: recipient.address, contactId: subject.entity_id }]
      : [];
  });
}

/** One question for each contact the preview asks about, above the verdict. */
export function CountryQuestion({
  preview,
}: Readonly<{ preview: Preview | undefined }>) {
  return (
    <>
      {countriesAskedFor(preview).map((asked) => (
        <CountryAsk key={asked.contactId} asked={asked} />
      ))}
    </>
  );
}

class CountryAlreadySet extends Error {}

function CountryAsk({ asked }: Readonly<{ asked: Asked }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const [country, setCountry] = useState("");
  const options = countryOptions(locale).map((option) => ({
    value: option.value.toUpperCase(),
    label: option.label,
  }));
  const save = useMutation({
    mutationFn: async ({ country }: Readonly<{ country: string }>) => {
      // The address is written whole, so the rest of it is read first.
      const contact = unwrap(
        await api.GET("/contacts/{id}", {
          params: { path: { id: asked.contactId } },
        }),
      );
      // The preview asked because no country was stored. One stored since is
      // not overwritten with an answer to a question that is no longer true.
      if (contact.address?.country) {
        throw new CountryAlreadySet();
      }
      if (contact.version === undefined) {
        throw new Error("the contact read carries no version to write against");
      }
      return unwrap(
        await api.PATCH("/contacts/{id}", {
          params: {
            path: { id: asked.contactId },
            ...ifMatch(contact.version),
          },
          body: { address: { ...(contact.address ?? {}), country } },
        }),
      );
    },
    onSettled: () =>
      client.invalidateQueries({ queryKey: [...SEND_PERMISSION_KEY] }),
  });
  return (
    <Callout
      tone="info"
      kind="standing"
      live="status"
      title={t("countryQuestion.title", { address: asked.address })}
      actions={
        <Button
          disabled={country === ""}
          pending={save.isPending}
          onClick={() => save.mutate({ country })}
        >
          {t("countryQuestion.save")}
        </Button>
      }
    >
      <p className="t-body">{t("countryQuestion.body")}</p>
      <Field label={t("countryQuestion.label")}>
        {(control) => (
          <Select
            {...control}
            value={country}
            placeholder={t("countryQuestion.none")}
            options={options}
            onChange={setCountry}
          />
        )}
      </Field>
      {save.error ? (
        <p role="alert" className="t-caption">
          {save.error instanceof CountryAlreadySet
            ? t("countryQuestion.alreadySet")
            : problemMessageOf(save.error, t, t("countryQuestion.failed"))}
        </p>
      ) : null}
    </Callout>
  );
}
