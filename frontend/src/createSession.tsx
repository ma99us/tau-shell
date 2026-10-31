import { FormEvent, useEffect, useState } from "react";
import { app, LaunchPreview, Session } from "./bridge";
import { HarnessIcon } from "./harnessIcons";
import { harnesses } from "./harnesses";

function parseLaunchArgs(text: string): string[] {
  return text.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean);
}

export function CreateSession({ recentFolders, onClose, onCreated, onError }: {
  recentFolders: string[];
  onClose: () => void;
  onCreated: (session: Session) => void;
  onError: (error: unknown) => void;
}) {
  const [tool, setTool] = useState<Session["tool"]>("copilot-cli");
  const [workingDir, setWorkingDir] = useState("");
  const [launchArgs, setLaunchArgs] = useState("");
  const [templateText, setTemplateText] = useState<string | null>(null);
  const [preview, setPreview] = useState<LaunchPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [saving, setSaving] = useState(false);
  const [yamlOpen, setYamlOpen] = useState(false);
  const [yamlText, setYamlText] = useState("");
  const [yamlDirty, setYamlDirty] = useState(false);
  const [yamlError, setYamlError] = useState("");
  const chooseDirectory = async () => {
    try { const path = await app().SelectWorkingDirectory(); if (path) setWorkingDir(path); } catch (err) { onError(err); }
  };
  // Prefill with the harness-specific template on first load and whenever
  // the user switches tool while the field still matches the prior
  // template verbatim (i.e. they have not customized it yet).
  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const template = await app().GetSessionTemplate(tool);
        if (cancelled) return;
        const text = template.launchArgs.join("\n");
        setLaunchArgs((current) => (templateText === null || current === templateText ? text : current));
        setTemplateText(text);
      } catch (err) { if (!cancelled) onError(err); }
    })();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tool]);
  // Preview the effective executable and discrete argument list before
  // saving, including any launch argument TauShell cannot verify against
  // the installed harness's own --help output.
  useEffect(() => {
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const result = await app().PreviewSessionLaunch(tool, parseLaunchArgs(launchArgs));
          if (!cancelled) { setPreview(result); setPreviewError(""); }
        } catch (err) {
          if (!cancelled) { setPreview(null); setPreviewError(err instanceof Error ? err.message : String(err)); }
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [tool, launchArgs]);
  // The YAML view follows the form until the user edits it by hand; after
  // that their text is kept until they choose "Reset from form".
  useEffect(() => {
    if (!yamlOpen || yamlDirty) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const text = await app().PreviewSessionYAML("", tool, workingDir, parseLaunchArgs(launchArgs));
          if (!cancelled) { setYamlText(text); setYamlError(""); }
        } catch (err) {
          if (!cancelled) setYamlError(err instanceof Error ? err.message : String(err));
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [yamlOpen, yamlDirty, tool, workingDir, launchArgs]);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSaving(true);
    try {
      const session = yamlOpen
        ? await app().CreateSessionFromYAML(yamlText)
        : await app().CreateSession("", tool, workingDir, parseLaunchArgs(launchArgs));
      onCreated(session);
    } catch (err) { onError(err); } finally { setSaving(false); }
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal">
    <div className="modal-heading"><h2>Start new AI session</h2><button className="icon-button" onClick={onClose}>x</button></div>
    <form onSubmit={(event) => void submit(event)}>
      <label>Project folder<div className="folder-row"><input autoFocus required list="recent-project-folders" value={workingDir} onChange={(event) => setWorkingDir(event.target.value)} placeholder="Choose a local project folder" /><datalist id="recent-project-folders">{recentFolders.map((folder) => <option key={folder} value={folder} />)}</datalist><button type="button" className="secondary-button" onClick={() => void chooseDirectory()}>Browse</button></div></label>
      <div className="harness-field"><span className="harness-label" id="harness-label">AI harness</span>
        <div className="harness-picker" role="radiogroup" aria-labelledby="harness-label">
          {harnesses.map(({ id, name }) =>
            <button key={id} type="button" role="radio" aria-checked={tool === id} className={`harness-option ${tool === id ? "selected" : ""}`} onClick={() => setTool(id)}>
              <HarnessIcon tool={id} /><span className="harness-name">{name}</span>
            </button>)}
        </div>
      </div>
      <label>Launch arguments <span className="optional">(editable template, one per line)</span><textarea rows={3} value={launchArgs} onChange={(event) => setLaunchArgs(event.target.value)} placeholder="--yolo" /></label>
      <div className="launch-preview">
        {previewError ? <p className="tool-warning">{previewError}</p> : preview && <>
          <p className="muted">Will run:</p>
          <code className="launch-command">{[preview.executable, ...preview.arguments].join(" ")}</code>
          {preview.warnings.map((warning) => <p key={warning} className="tool-warning">{warning}</p>)}
        </>}
      </div>
      <div className="yaml-section">
        <button type="button" className="text-button" aria-expanded={yamlOpen} onClick={() => setYamlOpen((open) => !open)}>{yamlOpen ? "Hide session YAML" : "Edit session YAML"}</button>
        {yamlOpen && <>
          <textarea className="yaml-editor" rows={9} spellCheck={false} aria-label="Session YAML" value={yamlText} onChange={(event) => { setYamlText(event.target.value); setYamlDirty(true); }} />
          {yamlError && <p className="tool-warning">{yamlError}</p>}
          <p className="form-note">{yamlDirty ? "Edited by hand: the form fields above no longer change this text. " : "Follows the form until you edit it. "}The definition is validated and saved in normal form.{yamlDirty && <> <button type="button" className="text-button" onClick={() => setYamlDirty(false)}>Reset from form</button></>}</p>
        </>}
      </div>
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={saving}>{saving ? "Creating..." : "Start new AI session"}</button></div>
    </form>
  </div></div>;
}
