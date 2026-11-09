// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { QueryGate } from "../../design-system/querystates";
import { SetPicker } from "../../design-system/setpicker";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

type Source = components["schemas"]["InternalMailSourceOption"];

export function InternalMailFolders({
  source,
  values,
  disabled,
  onChange,
}: Readonly<{
  source: Source;
  values: readonly string[];
  disabled: boolean;
  onChange: (values: string[]) => void;
}>) {
  const t = useT();
  const [load, setLoad] = useState(false);
  const provider =
    source.provider === "gmail" ||
    source.provider === "graph" ||
    source.provider === "imap"
      ? source.provider
      : undefined;
  const query = useQuery({
    queryKey: [
      "internal-mail-folders",
      source.connection_id,
      source.generation,
    ],
    enabled: load && provider !== undefined,
    queryFn: async () => {
      if (!provider) throw new Error(t("internalMail.foldersUnsupported"));
      const result = await api.GET("/connectors/{provider}/containers", {
        params: {
          path: { provider },
          query: { connection_id: source.connection_id },
        },
      });
      if (result.response.status === 501)
        return { containers: [], truncated: false, unsupported: true };
      return { ...unwrap(result), unsupported: false };
    },
  });
  return (
    <div>
      <p>
        {source.account_label || source.owner_name}:{" "}
        {values.length === 0
          ? t("internalMail.allFolders")
          : t("internalMail.selectedFolders")}
      </p>
      {!load ? (
        <Button disabled={disabled} onClick={() => setLoad(true)}>
          {t("internalMail.chooseFolders")}
        </Button>
      ) : (
        <QueryGate
          query={query}
          pendingLabel={t("internalMail.loadingFolders")}
        >
          {(data) =>
            data.unsupported ? (
              <p>{t("internalMail.foldersUnsupported")}</p>
            ) : (
              <>
                <SetPicker
                  legend={`${t("internalMail.folders")}: ${source.account_label || source.owner_name}`}
                  options={[
                    ...data.containers.map((folder) => ({
                      value: `${source.provider}:${folder.id}`,
                      label: folder.name,
                    })),
                    ...values
                      .filter(
                        (value) =>
                          !data.containers.some(
                            (folder) =>
                              `${source.provider}:${folder.id}` === value,
                          ),
                      )
                      .map((value) => ({
                        value,
                        label: t("internalMail.unavailableFolder"),
                      })),
                  ]}
                  values={values}
                  onChange={onChange}
                  disabled={disabled}
                  none={t("internalMail.allFolders")}
                  addLabel={t("internalMail.chooseFolders")}
                  removeLabel={(item) =>
                    `${t("internalMail.removeFolder")}: ${item.label}`
                  }
                  searchLabel={t("internalMail.searchFolders")}
                  noMatch={t("internalMail.noFolders")}
                  doneLabel={t("internalMail.done")}
                />
                {data.truncated && <p>{t("internalMail.partialFolders")}</p>}
              </>
            )
          }
        </QueryGate>
      )}
    </div>
  );
}
