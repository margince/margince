import { api } from "../api/client";
import { Checkbox } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";
import type { useTaskUpdate } from "./taskwrite";

export function TaskCompletionChoice({
  enabled,
  checked,
  onChange,
  pending,
}: Readonly<{
  enabled: boolean;
  checked: boolean;
  onChange: (checked: boolean) => void;
  pending: boolean;
}>) {
  const t = useT();
  return enabled ? (
    <Checkbox
      label={t("tasks.log.markDone")}
      checked={checked}
      onChange={(event) => onChange(event.target.checked)}
      disabled={pending}
    />
  ) : null;
}

// The activity is logged and only the task's completion failed, so a retry
// closes the task and logs nothing new.
export class TaskLeftOpenError extends Error {
  constructor(cause: unknown) {
    super("the activity was logged, the task is still open", { cause });
  }
}

// Read first: the task may be done already, and its version may have moved
// since the form opened.
export async function completeLoggedTask(
  update: ReturnType<typeof useTaskUpdate>,
  taskId: string,
  t: (key: MessageKey) => string,
): Promise<void> {
  try {
    const current = unwrap(
      await api.GET("/activities/{id}", { params: { path: { id: taskId } } }),
      t,
    );
    if (current.is_done) return;
    await update.mutateAsync({
      id: taskId,
      version: current.version,
      body: { is_done: true },
    });
  } catch (error) {
    throw new TaskLeftOpenError(error);
  }
}

export function LogErrorLine({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  return error instanceof TaskLeftOpenError ? (
    <ErrorLine>{t("tasks.log.completeFailed")}</ErrorLine>
  ) : (
    <ErrorLine error={error} />
  );
}
