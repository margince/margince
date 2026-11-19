// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { readKindRecord, useRecordKindDirectory } from "../lib/recordkinds";

/**
 * The name of the record a "Pick a record" field holds, read off the kind the
 * field points at. Until the name arrives, or when this reader may not read the
 * kind, it shows nothing rather than the id.
 */
export function KindRecordName({
  kind,
  id,
}: Readonly<{ kind: string; id: string }>) {
  const directory = useRecordKindDirectory().data?.data;
  const record = useQuery({
    queryKey: ["field-record", kind, id],
    enabled: directory !== undefined,
    queryFn: () => readKindRecord(kind, directory ?? [], id),
  });
  return <>{record.data?.name ?? ""}</>;
}
