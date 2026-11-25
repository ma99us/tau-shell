import { useEffect, useState } from "react";
import { app } from "./bridge";
import { Icon } from "./icons";

// Opens Windows Voice typing (Win+H) for the focused terminal. Windows owns the
// panel and its listening state, so this state only tracks what TauShell asked for.
export function VoiceButton({ sessionId, focus, onError }: { sessionId: string; focus: () => void; onError: (error: unknown) => void }) {
  const [listening, setListening] = useState(false);
  useEffect(() => setListening(false), [sessionId]);
  const toggle = () => {
    focus();
    app().ToggleVoiceTyping()
      .then(() => setListening(!listening))
      .catch((error) => { setListening(false); onError(error); });
  };
  const title = listening
    ? "Voice typing is on (Windows panel). Click to stop; TauShell cannot confirm Windows is still listening."
    : "Voice typing: dictate into the prompt with Windows Voice typing (Win+H)";
  return <button className={`text-button terminal-action icon-button voice-button${listening ? " listening" : ""}`} aria-pressed={listening} title={title} aria-label={title} onClick={toggle}><Icon name="mic" /></button>;
}