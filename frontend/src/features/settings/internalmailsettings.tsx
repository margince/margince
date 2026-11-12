// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import {
  useCaptureSettings,
  useUpdateCaptureSettings,
} from "../../components/capture-settings";
import { Checkbox } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { useSaveDraft } from "../../design-system/savepage";
import { useT } from "../../i18n";
import { InternalMailFolders } from "./internalmailfolders";
import { InternalMailHistoryScope } from "./internalmailhistoryscope";

type Settings = components["schemas"]["InternalMailSettings"];
type Draft = components["schemas"]["UpdateInternalMailRequest"];

export function InternalMailCaptureCard() {
  const query = useCaptureSettings();
  return query.data?.internal_mail ? (
    <InternalMailEditor settings={query.data.internal_mail} />
  ) : null;
}

function InternalMailEditor({ settings }: Readonly<{ settings: Settings }>) {
  const t = useT();
  const update = useUpdateCaptureSettings();
  const saved: Draft = {
    enabled: settings.enabled,
    sources: settings.sources.map(
      ({ connection_id, containers, history_since }) => ({
        connection_id,
        containers,
        history_since,
      }),
    ),
  };
  const [draft, setDraft] = useState<Draft | null>(null);
  const value = draft ?? saved;
  const missingSource = value.enabled && value.sources.length === 0;
  const missingHistoryDate = value.sources.some(
    (source) => source.history_since === "",
  );
  const invalid = missingSource || missingHistoryDate;
  const reenroll =
    value.enabled &&
    settings.sources.some(
      (old) =>
        value.sources.some(
          (source) => source.connection_id === old.connection_id,
        ) &&
        settings.available_sources.some(
          (current) =>
            current.connection_id === old.connection_id &&
            current.generation !== old.generation,
        ),
    );
  useSaveDraft({
    dirty: JSON.stringify(value) !== JSON.stringify(saved),
    offered: reenroll,
    offer: settings.available_sources
      .map((source) => `${source.connection_id}:${source.generation}`)
      .join(","),
    invalid,
    save: async () => {
      await update.mutateAsync({ internal_mail: value });
      setDraft(null);
    },
    discard: () => setDraft(null),
  });
  const available = settings.available_sources.map((source) => ({
    value: source.connection_id,
    label: source.account_label || source.owner_name,
    description: `${source.owner_user_id === null ? t("internalMail.sharedSource") : source.owner_name} · ${source.provider}`,
  }));
  const choices = [
    ...available,
    ...settings.sources
      .filter(
        (source) =>
          !available.some((option) => option.value === source.connection_id),
      )
      .map((source) => ({
        value: source.connection_id,
        label: t("internalMail.unavailableSource"),
        description: source.provider,
      })),
  ];
  return (
    <Panel title={t("internalMail.title")}>
      <PanelBody className="form-stack">
        <p>{t("internalMail.scope")}</p>
        <p>{t("internalMail.authorization")}</p>
        <Checkbox
          label={t("internalMail.enable")}
          checked={value.enabled}
          disabled={update.isPending}
          onChange={() => setDraft({ ...value, enabled: !value.enabled })}
        />
        {choices.length > 0 ? (
          <ChoiceList
            multiple
            legend={t("internalMail.sources")}
            value={value.sources.map((source) => source.connection_id)}
            choices={choices}
            disabled={update.isPending}
            onChange={(ids) =>
              setDraft({
                ...value,
                sources: ids.map(
                  (connection_id) =>
                    value.sources.find(
                      (source) => source.connection_id === connection_id,
                    ) ?? { connection_id, containers: [] },
                ),
              })
            }
          />
        ) : (
          <p>{t("internalMail.noSources")}</p>
        )}
        {value.sources.map((selection) => {
          const source = settings.available_sources.find(
            (source) => source.connection_id === selection.connection_id,
          );
          return source ? (
            <section
              key={selection.connection_id}
              className="form-stack"
              aria-label={source.account_label || source.owner_name}
            >
              <InternalMailFolders
                source={source}
                values={selection.containers}
                disabled={update.isPending}
                onChange={(containers) =>
                  setDraft({
                    ...value,
                    sources: value.sources.map((item) =>
                      item.connection_id === selection.connection_id
                        ? { ...item, containers }
                        : item,
                    ),
                  })
                }
              />
              {source.owner_user_id === null ? (
                <p className="t-caption">{t("internalMail.sharedNoHistory")}</p>
              ) : (
                <InternalMailHistoryScope
                  value={selection.history_since}
                  disabled={update.isPending}
                  onChange={(history_since) =>
                    setDraft({
                      ...value,
                      sources: value.sources.map((item) =>
                        item.connection_id === selection.connection_id
                          ? { ...item, history_since }
                          : item,
                      ),
                    })
                  }
                />
              )}
            </section>
          ) : null;
        })}
        {missingSource && <ErrorLine>{t("internalMail.needSource")}</ErrorLine>}
        {missingHistoryDate && (
          <ErrorLine>{t("internalMail.needHistoryDate")}</ErrorLine>
        )}
        {reenroll && <p>{t("internalMail.reauthorizeSource")}</p>}
        <p className="t-caption">{t("internalMail.disable")}</p>
        <p className="t-caption">{t("internalMail.future")}</p>
      </PanelBody>
    </Panel>
  );
}
