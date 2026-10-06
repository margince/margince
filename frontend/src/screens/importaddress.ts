// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useUnsavedGuard } from "../app/unsaved";
import { currentParams, replaceParams, useUrlParams } from "../app/urlstate";
import { useImportFlow } from "./importflow";
import {
  type ImportObject,
  isImportObject,
  UNNAMED_IMPORT_OBJECT,
} from "./importtypes";

const OBJECT_PARAM = "object";

function nameObject(object: ImportObject) {
  replaceParams(new Map(currentParams()).set(OBJECT_PARAM, object));
}

// A move that starts the flow over, and so throws away a profiled file.
type Restart =
  | Readonly<{ kind: "object"; next: ImportObject }>
  | Readonly<{ kind: "file"; file: File }>;

// The run page's flow. `?object=` and the row type on screen agree both ways:
// the address moving changes the type, and the type changing writes the address,
// so a reload, a shared link or Back keeps it.
export function useAddressedImportFlow() {
  const [params] = useUrlParams();
  const named = params.get(OBJECT_PARAM) ?? UNNAMED_IMPORT_OBJECT;
  const flow = useImportFlow(
    isImportObject(named) ? named : UNNAMED_IMPORT_OBJECT,
  );
  const { run, upload, validate, commit, undo } = flow;
  const committed =
    run?.status === "complete" ||
    run?.status === "failed" ||
    run?.status === "undoing" ||
    run?.status === "undone";
  const unsaved = (flow.profile !== null || upload.isPending) && !committed;
  useUnsavedGuard(unsaved);
  // Leaving keeps a resumable run's remembered id; starting over forgets it.
  const holdsWork =
    unsaved || run?.status === "failed" || run?.status === "undoing";
  const [asking, setAsking] = useState<Restart | null>(null);

  const perform = (restart: Restart) => {
    if (restart.kind === "file") {
      upload.mutate(restart.file);
      return;
    }
    flow.chooseObject(restart.next);
    nameObject(restart.next);
  };
  const request = (restart: Restart) =>
    holdsWork ? setAsking(restart) : perform(restart);

  const adopt = useEffectEvent((next: ImportObject) =>
    request({ kind: "object", next }),
  );
  const seen = useRef(named);
  useEffect(() => {
    const moved = seen.current !== named;
    seen.current = named;
    if (named === flow.object) {
      return;
    }
    if (moved && isImportObject(named)) {
      adopt(named);
      return;
    }
    // A typo, or a recovered run of another type.
    nameObject(flow.object);
  }, [named, flow.object]);

  return {
    ...flow,
    busy:
      upload.isPending ||
      validate.isPending ||
      commit.isPending ||
      undo.isPending,
    committed,
    chooseObject: (next: ImportObject) => {
      if (next !== flow.object) {
        request({ kind: "object", next });
      }
    },
    chooseFile: (file: File) => request({ kind: "file", file }),
    asking: asking !== null,
    keepWork: () => {
      setAsking(null);
      if (named !== flow.object) {
        nameObject(flow.object);
      }
    },
    discardWork: () => {
      setAsking(null);
      if (asking) {
        perform(asking);
      }
    },
  };
}
