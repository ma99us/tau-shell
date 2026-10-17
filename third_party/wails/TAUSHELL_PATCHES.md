# TauShell Wails fork

This directory contains the upstream Wails v2.16.0 module. Its `LICENSE`
applies to the Wails source. The nested `internal/go-webview2` module is
upstream `github.com/wailsapp/go-webview2` v1.0.22 and retains its own license.

TauShell's changes are limited to:

- Adding `options.App.OnNavigationStarting` and wiring it through the Windows
  frontend.
- Exposing WebView2's native `NavigationStarting` event in the `edge.Chromium`
  wrapper, including its URI and cancellation APIs.

The root `go.mod` replaces both upstream modules with these local copies. Keep
the upstream version markers and licenses intact when refreshing the fork, and
reapply the navigation callback patch.
