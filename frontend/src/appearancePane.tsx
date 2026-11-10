import { HomePanel } from "./homePanel";
import { saveAppearance, useAppearance } from "./theme/appearance";
import { clampFontScale, clampTerminalFont, fontScale, percent, terminalFont, ThemeChoice, themeChoices } from "./theme/appearanceModel";

function Stepper({ label, value, text, min, max, onChange, onReset, resetTitle }: {
  label: string;
  value: number;
  text: string;
  min: number;
  max: number;
  onChange: (direction: 1 | -1) => void;
  onReset: () => void;
  resetTitle: string;
}) {
  return <div className="appearance-row">
    <span className="appearance-label">{label}</span>
    <div className="stepper" role="group" aria-label={label}>
      <button className="secondary-button" aria-label={`Decrease ${label.toLowerCase()}`} disabled={value <= min} onClick={() => onChange(-1)}>-</button>
      <span className="stepper-value" aria-live="polite">{text}</span>
      <button className="secondary-button" aria-label={`Increase ${label.toLowerCase()}`} disabled={value >= max} onClick={() => onChange(1)}>+</button>
      <button className="link-button" title={resetTitle} onClick={onReset}>Reset</button>
    </div>
  </div>;
}

// The Home panel for the theme and text sizes. The theme and UI text size apply
// everywhere at once; running harnesses keep their own colors until restarted.
export function AppearancePane({ onError }: { onError: (source: string, error: unknown) => void }) {
  const { appearance } = useAppearance();
  const save = (next: typeof appearance) => saveAppearance(next, onError);
  const withTheme = (theme: ThemeChoice) => save({ ...appearance, theme });
  return <HomePanel className="appearance-pane" title="Appearance"
    description="Theme and text sizes apply at once and are saved to settings. Running harnesses keep their own colors until restarted; most have their own light/dark theme setting (for example /theme in Claude Code and Copilot CLI).">
    <div className="appearance-row">
      <span className="appearance-label">Theme</span>
      <div className="segmented" role="group" aria-label="Theme">
        {themeChoices.map((choice) => <button key={choice.id} className={`segment${appearance.theme === choice.id ? " active" : ""}`} aria-pressed={appearance.theme === choice.id} onClick={() => withTheme(choice.id)}>{choice.label}</button>)}
      </div>
    </div>
    <Stepper label="UI text size" value={appearance.fontScale} text={percent(appearance.fontScale)} min={fontScale.min} max={fontScale.max}
      onChange={(direction) => save({ ...appearance, fontScale: clampFontScale(appearance.fontScale + direction * fontScale.step) })}
      onReset={() => save({ ...appearance, fontScale: fontScale.default })} resetTitle="Back to 100%" />
    <Stepper label="Terminal text size" value={appearance.terminalFontSize} text={`${appearance.terminalFontSize} px`} min={terminalFont.min} max={terminalFont.max}
      onChange={(direction) => save({ ...appearance, terminalFontSize: clampTerminalFont(appearance.terminalFontSize + direction * terminalFont.step) })}
      onReset={() => save({ ...appearance, terminalFontSize: terminalFont.default })} resetTitle="Back to 14 px" />
  </HomePanel>;
}
