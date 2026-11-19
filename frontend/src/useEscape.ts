import { useEffect, useRef } from "react";

// Open dialogs, oldest first. Escape closes only the newest, so a confirmation
// opened over another dialog does not dismiss both.
const open: symbol[] = [];

// Calls onClose when Escape is pressed while this dialog is the topmost one.
// Pass enabled=false while the dialog must not be dismissed (work in progress).
export function useEscape(onClose: () => void, enabled = true) {
  const latest = useRef(onClose);
  latest.current = onClose;
  useEffect(() => {
    if (!enabled) return;
    const id = Symbol("dialog");
    open.push(id);
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || open[open.length - 1] !== id) return;
      event.preventDefault();
      latest.current();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      const index = open.indexOf(id);
      if (index >= 0) open.splice(index, 1);
    };
  }, [enabled]);
}
