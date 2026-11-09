import { useEffect, useRef, useState } from "react";
import { addToast, clearToasts, dismissToast, toastsFor, type Toast, type ToastTone } from "./toasts";

export interface ToastOptions {
  // Milliseconds before the toast disappears on its own; omitted means it stays until dismissed.
  ttl?: number;
}

export interface Toasts {
  all: Toast[];
  notify: (scope: string, tone: ToastTone, message: string, options?: ToastOptions) => void;
  dismiss: (id: number) => void;
  clear: (scope: string, tone?: ToastTone) => void;
}

export function useToasts(): Toasts {
  const [all, setAll] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, number>());
  useEffect(() => () => timers.current.forEach((timer) => window.clearTimeout(timer)), []);

  const dismiss = (id: number) => {
    window.clearTimeout(timers.current.get(id));
    timers.current.delete(id);
    setAll((current) => dismissToast(current, id));
  };
  const notify: Toasts["notify"] = (scope, tone, message, options = {}) => {
    const id = nextId.current++;
    setAll((current) => addToast(current, { id, scope, tone, message }));
    if (options.ttl) timers.current.set(id, window.setTimeout(() => dismiss(id), options.ttl));
  };
  const clear: Toasts["clear"] = (scope, tone) => setAll((current) => clearToasts(current, scope, tone));
  return { all, notify, dismiss, clear };
}

// Shows the toasts of one scope. Put it where those toasts should appear: above the whole
// app for the app scope, or inside a session's panel for that session.
export function ToastStack({ toasts, scope }: { toasts: Toasts; scope: string }) {
  return <>
    {toastsFor(toasts.all, scope).map((toast) => (
      <div key={toast.id} className={toast.tone === "error" ? "error-banner" : "notice-banner"} role={toast.tone === "error" ? "alert" : "status"}>
        {toast.message}<button onClick={() => toasts.dismiss(toast.id)}>Dismiss</button>
      </div>
    ))}
  </>;
}
