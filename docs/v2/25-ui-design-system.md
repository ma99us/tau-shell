# UI design system

Status: describes the shipped Phase 1 desktop UI (React in WebView2). The
tokens live in code; this document explains them and the GUI decisions behind
the screens. If the two disagree, fix whichever is wrong in the same change.

## Where the theme lives

| File | Role |
|---|---|
| `frontend/src/theme/tokens.css` | Every color, type size, radius and font as a CSS custom property on `:root`, including terminal, brand-mark and session tag colors. Values only, no selectors. |
| `frontend/src/theme/theme.ts` | JavaScript access to those tokens (xterm options, tag swatches, brand fills). It reads the CSS variables at runtime and contains no color or size literals. |
| `frontend/src/style.css` | Layout and components. It must not contain raw color, font-size or border-radius literals; use tokens. |
| `frontend/scripts/check-tokens.mjs` | `npm run check:tokens` fails on hard-coded colors, font sizes or radii anywhere in `src/` outside `tokens.css`. Run it before committing UI changes. |

Rules for changes:

- Never hard-code colors, font sizes or radii in screen code (`main.tsx`,
  components, `style.css`). Always use a token.
- Add a token before using a new color. Reuse the nearest existing one first.
- Need a value in JavaScript? Add a token and read it through `theme.ts`.
- Per-session colors are the one runtime theme input. A session tag sets
  `--session-color` inline, and the CSS mixes it with `color-mix()` against
  `--bg-panel`, `--white` and `--text-3`. Text on a tag color is chosen by
  luminance in `readableTextColor`.
- TauShell-owned UI text is ASCII (`~$`, `k`, `M`, `x`, `|`). Provider output
  in the terminal is never rewritten.
- No `target=_blank`; links go through the host.

## Theme: dark only

`color-scheme: dark`. There is no light theme or user theme setting in Phase 1;
the token layer is the seam for adding one later (swap the `:root` values).

### Tokens

Neutral surfaces (darkest to lightest): `--bg-app` (window and terminal),
`--bg-inset`, `--bg-panel` (sidebar, tab strip, empty cards), `--bg-card`
(session tiles), `--bg-raised`, `--bg-hover`, `--bg-control`,
`--bg-control-hover`, `--bg-elevated`. Overlays: `--scrim`, `--shadow-soft`,
`--shadow-deep`.

Borders: `--border-subtle` (structural dividers, 1px), `--border` (cards),
`--border-strong` (dashed empty state, controls), `--border-hover`,
`--border-muted`, `--border-control-hover`.

Text ramp: `--white`, `--text-1` (strongest, hover) to `--text-11` (dimmest).
Typical use: `--text-2` body, `--text-title` headings, `--text-7` muted copy,
`--text-9` and dimmer for small labels and meta.

Accent (green) is the brand color. `--accent` is the primary button and focus
family; `--action` is the bright green tile button (Resume/Start). Active
navigation, tabs and selected items use the dark green ramp
(`--accent-bg-*`, `--accent-border-*`, `--accent-text-*`).

Status colors, always paired with a shape or label, never color alone:

| Token | Meaning |
|---|---|
| `--status-ok` | running / healthy (green dot) |
| `--status-warn` | warning (yellow dot) |
| `--status-unseen` | finished, not yet seen (pulsing yellow circle) |
| `--status-input` | waiting for you (orange **square**, pulsing when unseen) |
| `--status-failed` | failed (red) |
| `--drop-indicator` | drag-and-drop insertion line (blue) |

Danger (delete, errors, failed notices) has its own text, background, border
and solid ramps (`--danger-*`).

### Type

- UI font `--font-ui` ("Segoe UI", Inter, sans-serif), base 14px on `:root`.
- Monospace `--font-mono` for paths, commands and toggles. The terminal uses
  `Cascadia Code, Consolas, monospace` at 14px (`theme.fontMono`).
- Scale `--fs-10` to `--fs-22`. Most chrome is 12px; small meta and labels
  are 10-11px; card and modal titles 15-16px; the Home title is
  `clamp(25px, 3vw, 34px)` at weight 650 with `-0.035em` tracking.
- Uppercase labels use `--tracking-label` (0.12em); the eyebrow uses
  `--tracking-eyebrow` (0.13em).

### Radius and spacing

Radius scale `--radius-2` to `--radius-20`. Buttons and tabs 6px, cards 9px,
empty cards 10px, modals larger, status dots 50%. Tabs round only the top
corners. Spacing is plain px in `style.css` (no spacing tokens yet); common
gaps are 8, 10 and 13px, card padding 15px, Home padding `34px 36px 42px`.

## The eyebrow style

A small, bold, wide-tracked, uppercase, green label above a heading. It was
used on the Home hero as "LOCAL SESSION WORKSPACE"; the owner likes the style
but the Home text was removed (the hero is now just the title and subtitle).
The style remains in use above modal titles ("NEW WORKSPACE", "RENAME
SESSION", "DELETE SESSION", "CLOSE SESSION TAB" and dialog notices).

```css
.eyebrow {
  margin: 0 0 10px;
  color: var(--accent-eyebrow);   /* #83d5a7 */
  font-size: var(--fs-10);        /* 10px */
  font-weight: 700;
  letter-spacing: var(--tracking-eyebrow); /* 0.13em */
}
```

Text is written in uppercase in the source (no CSS `text-transform`), and
stays ASCII. Use it for a short category label directly above an `h1` or `h2`,
never for body text or inside buttons.

## Layout

- **Shell:** a sidebar (244px, collapsible to a 58px icon rail, remembered in
  `localStorage`) and a main area with a 42px tab strip over the content.
  Below 820px the sidebar narrows to 205px; below 620px chrome compacts.
- **Sidebar:** brand row, a Home nav row, then session rows. Each session row
  has a state icon, the name and a small second line. Rows can be reordered by
  drag; the blue drop line shows the target. Active rows use the green ramp.
- **Tabs:** persistent session tabs next to a fixed Home tab; the strip
  scrolls with hidden scrollbars and arrow buttons. Active tab uses the green
  ramp. A tagged session colors its tab and sidebar row.
- **Home:** hero (title, subtitle, actions), the sessions header with the
  month usage totals, a grid of session tiles
  (`repeat(auto-fill, minmax(min(100%, 300px), 330px))`, 13px gap), then the
  tools panel.
- **Session tile:** project icon and state pill on top; the name line with
  total tokens and `~$` price pulled to the right (name ellipsizes, usage
  never wraps); path and meta; error text; actions (primary tile button plus
  text buttons).
- **Tools panels:** general tools first, then AI Harnesses in a separate panel
  underneath. Each tile has a non-wrapping name/version heading, executable
  path on the second line, optional warnings below, then optional install or
  update actions. Long names and paths ellipsize with full-text tooltips. The
  status dot is vertically centered on the left, opposite the config gear in
  a reserved right-hand column. Harness config gears keep their existing host
  action. An "Update to X" button appears only when winget reports one.
  Refresh in the Tools header refreshes both panels, with a spinner to its
  left; the button is disabled while refreshing and always re-enabled.
- **Session top bar:** `tool | path | branch | model | tokens | ~$cost`,
  with dim `|` separators (`.toolbar-sep`). A part with no value is dropped
  together with its separator. Status text and the harness widgets (Commands,
  Prompts, Model, Resume) sit on the right.
- **Notices:** the pending-command banner belongs to its session panel and is
  not shown on other tabs.
- **Modals and palette:** centered over `--scrim`; eyebrow, then `h2`;
  actions right-aligned; Escape closes. The command palette (`Ctrl+K`) is a
  grouped, keyboard-navigable list.

## Interaction decisions

- **Primary vs secondary:** one primary (green) action per surface; others are
  secondary or text buttons. Destructive actions use the danger button and a
  confirmation naming the session.
- **State is never color only:** running is a dot, "waiting for you" is a
  square, unseen is animated, failed is red with text.
- **Focus:** a 2px `--accent-focus` outline with 2px offset on all controls
  and tab panels. Disabled controls are 55% opacity with a not-allowed cursor.
- **Motion:** one spinner (`state-spin`, 0.9s) reused for working states and
  Refresh; glow pulses (1.4s) for unseen states; 120ms color transitions on
  buttons. No other animation.
- **Opening files:** settings, prompts and tool configs open in the system
  default editor through `ShellExecute` (notepad as fallback), never through a
  console window.
- **Honest numbers:** token and price figures are estimates; unpriced usage
  shows a trailing `+` or `$?`, and tooltips say "estimate".
- **Errors surface:** failures show a banner or notice; nothing is swallowed
  silently.

## Changing the look

1. Edit a value in `tokens.css` to re-tone everything that uses it.
2. For a new color, add a token with a role name, then use `var(--name)`.
3. If the value is also needed in JavaScript, mirror it in `theme.ts` with a
   comment naming the token.
4. Rebuild with `npm run build` in `frontend/`, then check Home, a session
   tab, the sidebar (expanded and collapsed) and a modal.
