// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { currentParams, replaceParams, useUrlParams } from "../app/urlstate";
import { useImportFlow } from "./importflow";
import { type ImportObject, isImportObject } from "./importtypes";

const OBJECT_PARAM = "object";

function nameObject(object: ImportObject) {
  replaceParams(new Map(currentParams()).set(OBJECT_PARAM, object));
}

// The run page's flow: it starts on the row type `?object=` names, and the
// address follows the type on screen so a reload or a shared link keeps it.
export function useAddressedImportFlow() {
  const [params] = useUrlParams();
  const asked = params.get(OBJECT_PARAM);
  const flow = useImportFlow(isImportObject(asked) ? asked : undefined);
  // A typo, or a recovered run of another type, would otherwise leave the
  // address naming rows the page is not importing.
  useEffect(() => {
    if (asked !== undefined && asked !== flow.object) {
      nameObject(flow.object);
    }
  }, [asked, flow.object]);
  return {
    ...flow,
    chooseObject: (next: ImportObject) => {
      flow.chooseObject(next);
      nameObject(next);
    },
  };
}
