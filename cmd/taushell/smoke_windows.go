//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/ma99us/tau-shell/internal/fsutil"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The end-to-end smoke test (scripts/smoke-test.ps1, e2e/smoke.mjs) sets these
// before launching the app. TAUSHELL_SMOKE_OUT is where the page's report is
// written; TAUSHELL_SMOKE_METHODS lists the bridge methods the UI declares.
const (
	smokeOutEnv       = "TAUSHELL_SMOKE_OUT"
	smokeMethodsEnv   = "TAUSHELL_SMOKE_METHODS"
	smokeLifecycleEnv = "TAUSHELL_SMOKE_LIFECYCLE"
)

// smokeScript waits for Home to render, lets late render errors surface, then
// reports what the page found through the bound SmokeReport method.
func smokeScript(methods []string) string {
	list, _ := json.Marshal(methods)
	return `(async () => {
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const app = window.go && window.go.main && window.go.main.App;
  const deadline = Date.now() + 60000;
  while (!document.querySelector('.home') && Date.now() < deadline) await sleep(200);
  await sleep(3000);
  const banner = document.querySelector('.error-banner');
  const report = {
    home: Boolean(document.querySelector('.home')),
    missing: ` + string(list) + `.filter((name) => !app || typeof app[name] !== 'function'),
    banner: banner ? banner.textContent : '',
    version: app && typeof app.GetAppVersion === 'function' ? await app.GetAppVersion() : '',
    displayedVersion: document.querySelector('.home-version')?.textContent || '',
  };
  if (app) await app.SmokeReport(JSON.stringify(report));
})();`
}

func (application *App) startSmokeTest(ctx context.Context) {
	if os.Getenv(smokeOutEnv) == "" {
		return
	}
	var methods []string
	for _, name := range strings.Split(os.Getenv(smokeMethodsEnv), ",") {
		if name = strings.TrimSpace(name); name != "" {
			methods = append(methods, name)
		}
	}
	if phase := os.Getenv(smokeLifecycleEnv); phase == "prepare" || phase == "restore" {
		runtime.WindowExecJS(ctx, smokeLifecycleScript(methods, phase))
		return
	}
	runtime.WindowExecJS(ctx, smokeScript(methods))
}

func smokeLifecycleScript(methods []string, phase string) string {
	methodList, _ := json.Marshal(methods)
	phaseValue, _ := json.Marshal(phase)
	return `(async () => {
	const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
	const app = window.go && window.go.main && window.go.main.App;
	const phase = ` + string(phaseValue) + `;
	const result = {
		phase,
		home: false,
		missing: ` + string(methodList) + `.filter((name) => !app || typeof app[name] !== 'function'),
		banner: document.querySelector('.error-banner')?.textContent || '',
		version: app && typeof app.GetAppVersion === 'function' ? await app.GetAppVersion() : '',
		displayedVersion: document.querySelector('.home-version')?.textContent || '',
		seedStarted: false,
		seedStopped: false,
		hasResumeId: false,
		autoRestored: false,
		confirmationCount: 0,
		shutdownFinished: false,
		allStopped: false,
		error: ''
	};
	const writeReport = async () => { if (app) await app.SmokeReport(JSON.stringify(result)); };
	const homeDeadline = Date.now() + 60000;
	while (!document.querySelector('.home') && Date.now() < homeDeadline) await sleep(200);
	await sleep(3000);
	result.home = Boolean(document.querySelector('.home'));
	result.banner = document.querySelector('.error-banner')?.textContent || '';
	const handledDialogs = new WeakSet();
	const answerShutdownPrompt = () => {
		for (const dialog of document.querySelectorAll('[role="alertdialog"]')) {
			if (dialog.querySelector('h2')?.textContent?.trim() !== 'Sessions are still running' || handledDialogs.has(dialog)) continue;
			handledDialogs.add(dialog);
			result.confirmationCount++;
			const yes = [...dialog.querySelectorAll('button')].find((button) => button.textContent.trim() === 'Yes');
			if (!yes) result.error = 'shutdown confirmation did not contain a Yes button';
			void writeReport();
			if (yes) setTimeout(() => yes.click(), 50);
		}
	};
	const observer = new MutationObserver(answerShutdownPrompt);
	observer.observe(document.body, { childList: true, subtree: true });
	window.runtime?.EventsOnMultiple('shutdown-progress', async (progress) => {
		if (progress.phase !== 'done') return;
		result.shutdownFinished = true;
		const sessions = await app.GetSessions();
		const fixture = sessions.find((session) => session.id === 'e2e-fixture');
		result.allStopped = !!fixture && !fixture.runningHere;
		await writeReport();
	}, -1);
	try {
		const deadline = Date.now() + 60000;
		if (phase === 'prepare') {
			const started = await app.StartSession('e2e-fixture', 80, 24);
			result.seedStarted = started.runningHere;
			result.hasResumeId = started.hasResumeId;
			await app.StopSession('e2e-fixture');
			const sessions = await app.GetSessions();
			const fixture = sessions.find((session) => session.id === 'e2e-fixture');
			result.seedStopped = !!fixture && !fixture.runningHere;
			result.hasResumeId = !!fixture?.hasResumeId;
			await writeReport();
		} else {
			let fixture;
			while (Date.now() < deadline) {
				const sessions = await app.GetSessions();
				fixture = sessions.find((session) => session.id === 'e2e-fixture');
				if (fixture?.runningHere || fixture?.lastError) break;
				await sleep(250);
			}
			result.autoRestored = !!fixture?.runningHere;
			result.hasResumeId = !!fixture?.hasResumeId;
			if (!result.autoRestored) result.error = fixture?.lastError || 'auto-resume did not start the fixture session';
			await writeReport();
		}
		await sleep(200);
		window.runtime?.Quit();
	} catch (error) {
		result.error = error instanceof Error ? error.message : String(error);
		await writeReport();
		window.runtime?.Quit();
	}
})();`
}

// SmokeReport stores the smoke-test page report. It does nothing unless the
// test harness set TAUSHELL_SMOKE_OUT.
func (application *App) SmokeReport(report string) error {
	out := os.Getenv(smokeOutEnv)
	if out == "" {
		return nil
	}
	return fsutil.ReplaceFile(out, []byte(report))
}
