import { FormEvent, useEffect, useRef, useState } from "react";
import { app, LaunchPreview, Session, WSLStatus, WorktreeEntry, WorktreePlan } from "./bridge";
import { HarnessIcon } from "./harnessIcons";
import { getHarnesses } from "./harnesses";
import { defaultDistro, isWindowsMount, runtimeLabel, wslChoices, wslHarnessSupported } from "./wslRuntime";

function parseLaunchArgs(text: string): string[] {
  return text.split(/\r?\n/).map((arg) => arg.trim()).filter(Boolean);
}

export function CreateSession({ recentFolders, wsl, powershellUsable = true, clone, onClose, onCreated, onError }: {
  recentFolders: string[];
  // Shared WSL readiness; the runtime can only be chosen for a new session.
  wsl?: WSLStatus | null;
  powershellUsable?: boolean;
  // Clone mode: the folder is inherited and fixed; harness and arguments start from the original's.
  clone?: { source: Session; launchArgs: string[] };
  onClose: () => void;
  onCreated: (session: Session) => void;
  // Returns the message to show inside the dialog; the page banner is hidden behind it.
  onError: (error: unknown) => string;
}) {
  const [tool, setTool] = useState<Session["tool"]>(clone?.source.tool ?? "copilot-cli");
  const [workingDir, setWorkingDir] = useState(clone?.source.workingDir ?? "");
  const [launchArgs, setLaunchArgs] = useState(clone ? clone.launchArgs.join("\n") : "");
  const toolChosen = useRef(false);
  // The runtime picker appears only when WSL reports distros. A clone keeps the original's runtime and distro, like its folder.
  const distros = wslChoices(wsl ?? null);
  const [runtime, setRuntime] = useState<"" | "wsl">(clone ? (clone.source.runtime === "wsl" ? "wsl" : "") : !powershellUsable && distros.length > 0 ? "wsl" : "");
  const [distro, setDistro] = useState(clone?.source.distro ?? (!powershellUsable ? defaultDistro(distros) : ""));
  const inWSL = runtime === "wsl";
  const runtimeName = inWSL ? "wsl" : "";
  const [templateText, setTemplateText] = useState<string | null>(null);
  const [preview, setPreview] = useState<LaunchPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");
  const fail = (err: unknown) => setFormError(onError(err));
  const [yamlOpen, setYamlOpen] = useState(false);
  const [yamlText, setYamlText] = useState("");
  const [yamlDirty, setYamlDirty] = useState(false);
  const [yamlError, setYamlError] = useState("");
  // Optional git worktree: a new folder and branch next to the repository.
  const [wtOn, setWtOn] = useState(false);
  const [wtPlan, setWtPlan] = useState<WorktreePlan | null>(null);
  const [wtError, setWtError] = useState("");
  const [wtName, setWtName] = useState("");
  const [wtBranch, setWtBranch] = useState("");
  const [wtBase, setWtBase] = useState("");
  const wtNameTouched = useRef(false);
  const wtBranchTouched = useRef(false);
  const [existing, setExisting] = useState<WorktreeEntry[]>([]);
  const chooseRuntime = (next: "" | "wsl") => {
    if (clone) return;
    setRuntime(next);
    setWorkingDir("");
    setFormError("");
    if (next === "wsl") {
      setDistro((current) => current || defaultDistro(distros));
      if (!wslHarnessSupported(tool)) { toolChosen.current = true; setTool("claude-code"); }
    }
  };
  const chooseDirectory = async () => {
    try { const path = inWSL ? await app().SelectWSLWorkingDirectory(distro) : await app().SelectWorkingDirectory(); if (path) { setWorkingDir(path); setFormError(""); } } catch (err) { fail(err); }
  };
  // Prefill with the harness-specific template on first load and whenever
  // the user switches tool while the field still matches the prior
  // template verbatim (i.e. they have not customized it yet).
  useEffect(() => {
    // A clone keeps the original's arguments until a different harness is picked.
    if (clone && !toolChosen.current) return;
    let cancelled = false;
    (async () => {
      try {
        const template = await app().GetSessionTemplate(tool);
        if (cancelled) return;
        const text = template.launchArgs.join("\n");
        setLaunchArgs((current) => (templateText === null || current === templateText ? text : current));
        setTemplateText(text);
      } catch (err) { if (!cancelled) fail(err); }
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
          const result = await app().PreviewSessionLaunchIn(runtimeName, inWSL ? distro : "", tool, parseLaunchArgs(launchArgs));
          if (!cancelled) { setPreview(result); setPreviewError(""); }
        } catch (err) {
          if (!cancelled) { setPreview(null); setPreviewError(err instanceof Error ? err.message : String(err)); }
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [tool, launchArgs, runtimeName, distro]);
  // The YAML view follows the form until the user edits it by hand; after
  // that their text is kept until they choose "Reset from form".
  useEffect(() => {
    if (!yamlOpen || yamlDirty) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const text = await app().PreviewSessionYAMLIn(runtimeName, inWSL ? distro : "", "", tool, workingDir, parseLaunchArgs(launchArgs));
          if (!cancelled) { setYamlText(text); setYamlError(""); }
        } catch (err) {
          if (!cancelled) setYamlError(err instanceof Error ? err.message : String(err));
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [yamlOpen, yamlDirty, tool, workingDir, launchArgs, runtimeName, distro]);
  // Worktree defaults: named after the folder (new) or the original session (clone).
  const wtSeed = clone ? clone.source.name : workingDir.split(/[\\/]+/).filter(Boolean).pop() ?? "";
  useEffect(() => {
    if (!wtOn || !workingDir.trim()) { setWtPlan(null); return; }
    let cancelled = false;
    const timer = setTimeout(() => {
      (async () => {
        try {
          const plan = await app().PlanWorktree(runtimeName, inWSL ? distro : "", workingDir, wtSeed);
          if (cancelled) return;
          setWtPlan(plan);
          setWtError("");
          if (!wtNameTouched.current) setWtName(plan.name);
          if (!wtBranchTouched.current) setWtBranch(plan.branch);
          setWtBase((current) => (current && plan.branches.includes(current) ? current : plan.base));
        } catch (err) {
          if (!cancelled) { setWtPlan(null); setWtError(err instanceof Error ? err.message : String(err)); }
        }
      })();
    }, 300);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [wtOn, workingDir, runtimeName, distro, wtSeed]);
  // Existing worktrees of the chosen repository can be adopted as the session folder.
  useEffect(() => {
    if (clone || !workingDir.trim()) { setExisting([]); return; }
    let cancelled = false;
    const timer = setTimeout(() => {
      app().ListWorktrees(runtimeName, inWSL ? distro : "", workingDir)
        .then((list) => { if (!cancelled) setExisting(list.filter((entry) => !entry.main && !entry.prunable)); })
        .catch(() => { if (!cancelled) setExisting([]); });
    }, 400);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [workingDir, runtimeName, distro]);
  const wtPath = wtPlan ? wtPlan.path.replace(/[^\\/]+$/, wtName) : "";
  const wtOptions = { name: wtName.trim(), branch: wtBranch.trim(), base: wtBase };
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setFormError("");
    setSaving(true);
    try {
      const session = clone
        ? wtOn
          ? await app().CloneSessionInWorktree(clone.source.id, tool, parseLaunchArgs(launchArgs), wtOptions)
          : await app().DuplicateSession(clone.source.id, tool, parseLaunchArgs(launchArgs))
        : yamlOpen
        ? await app().CreateSessionFromYAML(yamlText)
        : wtOn
        ? await app().CreateSessionInWorktree(runtimeName, inWSL ? distro : "", "", tool, workingDir, parseLaunchArgs(launchArgs), wtOptions)
        : await app().CreateSessionIn(runtimeName, inWSL ? distro : "", "", tool, workingDir, parseLaunchArgs(launchArgs));
      onCreated(session);
    } catch (err) { fail(err); } finally { setSaving(false); }
  };
  return <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}><div className="modal">
    <div className="modal-heading"><h2>{clone ? `Clone ${clone.source.name}` : "Start new AI session"}</h2><button className="icon-button" onClick={onClose}>x</button></div>
    <form onSubmit={(event) => void submit(event)}>
      {clone && <label>Runtime <span className="optional">(inherited from the original, cannot be changed)</span><div className="folder-row"><input readOnly value={runtimeLabel(clone.source)} /></div></label>}
      {!clone && distros.length > 0 && <div className="harness-field"><span className="harness-label" id="runtime-label">Runtime</span>
        <div className="harness-picker" role="radiogroup" aria-labelledby="runtime-label">
          <button type="button" role="radio" aria-checked={!inWSL} className={`harness-option ${!inWSL ? "selected" : ""}`} onClick={() => chooseRuntime("")}><span className="harness-name">PowerShell</span></button>
          <button type="button" role="radio" aria-checked={inWSL} className={`harness-option ${inWSL ? "selected" : ""}`} onClick={() => chooseRuntime("wsl")}><span className="harness-name">WSL</span></button>
        </div>
        {inWSL && <label>WSL distro<select value={distro} onChange={(event) => setDistro(event.target.value)}>{distros.map((item) => <option key={item.name} value={item.name}>{item.name}{item.default ? " (default)" : ""}</option>)}</select></label>}
      </div>}
      {clone
        ? <label>Project folder <span className="optional">(inherited from the original, cannot be changed)</span><div className="folder-row"><input readOnly value={workingDir} /></div></label>
        : <label>Project folder<div className="folder-row"><input autoFocus required list="recent-project-folders" value={workingDir} onChange={(event) => { setWorkingDir(event.target.value); setFormError(""); }} placeholder={inWSL ? "Linux path, for example /home/you/project" : "Choose a local project folder"} /><datalist id="recent-project-folders">{recentFolders.map((folder) => <option key={folder} value={folder} />)}</datalist><button type="button" className="secondary-button" onClick={() => void chooseDirectory()}>Browse</button></div>{inWSL && isWindowsMount(workingDir) && <span className="tool-warning">Projects under /mnt/c live on the Windows disk and are slower inside WSL; prefer a folder in the distro.</span>}</label>}
      {existing.length > 0 && !wtOn && <label>Use an existing worktree <span className="optional">(optional)</span>
        <select value="" onChange={(event) => { if (event.target.value) { setWorkingDir(event.target.value); setFormError(""); } }}>
          <option value="">Keep the folder above</option>
          {existing.map((entry) => <option key={entry.path} value={entry.path}>{entry.branch || "detached"} - {entry.path}</option>)}
        </select></label>}
      <div className="harness-field">
        <label className="check-option" title="Create a separate folder and branch for this session, so it cannot disturb your main checkout"><input type="checkbox" checked={wtOn} disabled={yamlOpen} onChange={(event) => setWtOn(event.target.checked)} />{clone ? "Clone into a new git worktree" : "Work in a new git worktree"}</label>
        {yamlOpen && <p className="form-note">Not available while editing the session YAML.</p>}
        {wtOn && <>
          {wtError && <p className="tool-warning">{wtError}</p>}
          {wtPlan && <>
            <label>Worktree name<input value={wtName} onChange={(event) => { wtNameTouched.current = true; setWtName(event.target.value); if (!wtBranchTouched.current) setWtBranch("taushell/" + event.target.value); }} /></label>
            <label>New branch<input value={wtBranch} onChange={(event) => { wtBranchTouched.current = true; setWtBranch(event.target.value); }} /></label>
            {wtPlan.branches.length > 0 && <label>Start from<select value={wtBase} onChange={(event) => setWtBase(event.target.value)}>{wtPlan.branches.map((branch) => <option key={branch} value={branch}>{branch}</option>)}</select></label>}
            <p className="form-note">Folder: {wtPath}</p>
            {wtPlan.dirty && <p className="tool-warning">The main checkout has uncommitted changes. They are not carried into the worktree.</p>}
          </>}
        </>}
      </div>
      <div className="harness-field"><span className="harness-label" id="harness-label">AI harness</span>
        <div className="harness-picker" role="radiogroup" aria-labelledby="harness-label">
          {getHarnesses().filter((item) => item.trusted && (!inWSL || wslHarnessSupported(item.id))).map(({ id, name }) =>
            <button key={id} type="button" role="radio" aria-checked={tool === id} className={`harness-option ${tool === id ? "selected" : ""}`} onClick={() => { toolChosen.current = true; setTool(id); }}>
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
      {!clone && <div className="yaml-section">
        <button type="button" className="text-button" aria-expanded={yamlOpen} onClick={() => setYamlOpen((open) => !open)}>{yamlOpen ? "Hide session YAML" : "Edit session YAML"}</button>
        {yamlOpen && <>
          <textarea className="yaml-editor" rows={9} spellCheck={false} aria-label="Session YAML" value={yamlText} onChange={(event) => { setYamlText(event.target.value); setYamlDirty(true); }} />
          {yamlError && <p className="tool-warning">{yamlError}</p>}
          <p className="form-note">{yamlDirty ? "Edited by hand: the form fields above no longer change this text. " : "Follows the form until you edit it. "}The definition is validated and saved in normal form.{yamlDirty && <> <button type="button" className="text-button" onClick={() => setYamlDirty(false)}>Reset from form</button></>}</p>
        </>}
      </div>}
      {formError && <p className="tool-warning" role="alert">{formError}</p>}
      <div className="modal-actions"><button type="button" className="secondary-button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={saving || (wtOn && (!wtPlan || !wtName.trim() || !wtBranch.trim()))}>{saving ? (clone ? "Cloning..." : "Creating...") : (clone ? "Clone and start" : "Start new AI session")}</button></div>
    </form>
  </div></div>;
}
