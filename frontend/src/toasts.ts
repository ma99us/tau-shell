// Toasts are short messages. A toast belongs to one scope: the whole app, or a single
// session. App toasts show above every tab; a session toast shows only on that session's
// panel, so it is visible exactly when its tab is selected.
export type ToastTone = "info" | "error";

export interface Toast {
  id: number;
  scope: string;
  tone: ToastTone;
  message: string;
}

// The scope of toasts about TauShell itself rather than one session.
export const APP_SCOPE = "";

// A scope shows one toast per tone, so a newer message replaces the older one instead of stacking.
export function addToast(toasts: Toast[], toast: Toast): Toast[] {
  return [...toasts.filter((current) => current.scope !== toast.scope || current.tone !== toast.tone), toast];
}

export function dismissToast(toasts: Toast[], id: number): Toast[] {
  return toasts.filter((toast) => toast.id !== id);
}

// Without a tone, every toast in the scope is cleared.
export function clearToasts(toasts: Toast[], scope: string, tone?: ToastTone): Toast[] {
  return toasts.filter((toast) => toast.scope !== scope || (tone !== undefined && toast.tone !== tone));
}

export function toastsFor(toasts: Toast[], scope: string): Toast[] {
  return toasts.filter((toast) => toast.scope === scope);
}
