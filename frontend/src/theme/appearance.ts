// Applies the appearance settings to the page and lets components follow them.
// config.yaml (through the Go host) is the source of truth; the last value is cached
// in localStorage only so the first paint after a restart already uses it.
import { useSyncExternalStore } from "react";
import { app } from "../bridge";
import { Appearance, appearanceKey, readCachedAppearance, ResolvedTheme, resolveTheme } from "./appearanceModel";

let current: Appearance = readCachedAppearance(localStorage);
let resolved: ResolvedTheme = "dark";
let version = 0;
const listeners = new Set<() => void>();
const systemQuery = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: dark)") : null;
let lastReported = "";

function apply(): void {
  resolved = resolveTheme(current.theme, systemQuery?.matches ?? true);
  const root = document.documentElement;
  root.dataset.theme = resolved;
  root.style.setProperty("--font-scale", String(current.fontScale));
  root.style.setProperty("--terminal-font-size", `${current.terminalFontSize}px`);
  document.querySelector('meta[name="color-scheme"]')?.setAttribute("content", resolved);
  version += 1;
  listeners.forEach((listener) => listener());
  if (lastReported !== resolved) {
    lastReported = resolved;
    try {
      void app().ReportTheme(resolved).catch(() => undefined);
    } catch {
      // The host bridge is not available (tests, plain browser); the hint is optional.
    }
  }
}

// Applies the cached value before the first render so there is no flash of the wrong theme.
export function initAppearance(): void {
  systemQuery?.addEventListener("change", () => { if (current.theme === "system") apply(); });
  apply();
}

// Replaces the settings (from the host or the Appearance panel) and applies them.
export function setAppearance(next: Appearance, remember = true): void {
  current = next;
  if (remember) {
    try {
      localStorage.setItem(appearanceKey, JSON.stringify(next));
    } catch {
      // The cache is only an optimization; the saved setting is in config.yaml.
    }
  }
  apply();
}

// Applies the new look at once and stores it through the host; a failed save restores the previous look and reports why.
export function saveAppearance(next: Appearance, onError: (source: string, error: unknown) => void): void {
  const previous = current;
  setAppearance(next);
  app().SetAppearance(next).catch((error) => { setAppearance(previous); onError("ui.appearance", error); });
}

export const appearanceNow = () => current;
export const resolvedThemeNow = () => resolved;

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
};

// Re-renders the caller whenever the theme or a text size changes.
export function useAppearance(): { appearance: Appearance; resolved: ResolvedTheme } {
  useSyncExternalStore(subscribe, () => version);
  return { appearance: current, resolved };
}
