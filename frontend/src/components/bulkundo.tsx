import { createContext, type ReactNode, Suspense, useState } from "react";
import { lazy } from "../i18n/lazy";
import type { BulkChangeRequest } from "./bulkchange";

const Dialog = lazy(async () => ({
  default: (await import("./bulkchange")).BulkChangeDialog,
}));
export const BulkUndoContext = createContext<
  ((requests: readonly BulkChangeRequest[]) => void) | null
>(null);

// The list's selection disappears after a change, so undo dialogs outlive it.
export function BulkUndoProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [queue, setQueue] = useState<readonly BulkChangeRequest[]>([]);
  const request = queue[0];
  return (
    <BulkUndoContext.Provider value={setQueue}>
      {children}
      {request && (
        <Suspense fallback={null}>
          <Dialog
            request={request}
            onClose={() => setQueue([])}
            onDone={() => setQueue((waiting) => waiting.slice(1))}
          />
        </Suspense>
      )}
    </BulkUndoContext.Provider>
  );
}
