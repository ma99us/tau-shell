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
	smokeOutEnv     = "TAUSHELL_SMOKE_OUT"
	smokeMethodsEnv = "TAUSHELL_SMOKE_METHODS"
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
	runtime.WindowExecJS(ctx, smokeScript(methods))
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
