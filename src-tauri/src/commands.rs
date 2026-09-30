// Copyright 2026 The Kstack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

use tauri::ipc::Channel;
use tauri::{AppHandle, Emitter, Manager, State, Webview};

use crate::cloud;
use crate::error::Result;
use crate::host_file::{self, HostFile, HostFilePatch};
use crate::services::sidecar::GraphqlResponse;
use crate::state::AppState;
use crate::webview_log::{self, WebviewRecord};

/// Probes the sidecar's IPC listener; the frontend's startup gate. `Err`
/// (frontend `networkError`) once the connect budget elapses.
#[tauri::command]
pub async fn ready(state: State<'_, AppState>) -> Result<()> {
    state.sidecar.ready().await
}

/// Opens a new application window (webview `AppMenu` "New Window" / `Ctrl+N`
/// on Linux/Windows; macOS uses its native menu, `app_menu.rs`).
///
/// Must stay `spawn_blocking`, never `spawn` or the main thread: `build()`
/// parks its thread until the webview exists — a main-thread build deadlocks
/// Windows (WebView2's controller is created by the main-thread event loop),
/// and parking an async worker would starve [`graphql_query`].
#[tauri::command]
pub async fn new_window(app: AppHandle) -> Result<()> {
    tauri::async_runtime::spawn_blocking(move || {
        app.state::<AppState>().window_manager.new_window(&app)?;
        Ok(())
    })
    .await
    .map_err(|err| std::io::Error::other(format!("window build task failed: {err}")))?
}

/// Applies a partial update to `host.json` (see `host_file`); adding a setting
/// extends `HostFilePatch`, not the command list. Broadcasts the merged file to
/// every window as `host_file::UPDATED_EVENT` and returns it. See
/// docs/adr/2026-08-09-host-json-settings.md.
#[tauri::command]
pub fn update_host_file(app: AppHandle, patch: HostFilePatch) -> Result<HostFile> {
    let file = host_file::update(&host_file::path(&app)?, patch)?;
    app.emit(host_file::UPDATED_EVENT, &file)?;
    Ok(file)
}

/// Quits via [`AppHandle::exit`] so the `RunEvent::ExitRequested` teardown
/// runs — the same path as the tray and macOS-menu "Quit".
#[tauri::command]
pub fn quit(app: AppHandle) {
    app.exit(0);
}

/// Opens the account page in the user's browser — the webview's avatar menu, the
/// same page the tray's account item opens.
///
/// Takes no URL. The webview holds the full cluster surface, so the destination
/// is the host's to name; a command that opened whatever it was handed would be
/// an exfiltration channel for anything executing in there.
#[tauri::command]
pub fn open_account_url(app: AppHandle) -> Result<()> {
    use tauri_plugin_opener::OpenerExt;
    app.opener().open_url(cloud::account_url(), None::<&str>)?;
    Ok(())
}

/// Writes one webview error record to the host's log (`src/lib/error-log.ts`).
/// Thin: the label is read off the `Webview` — never the payload, since the
/// label is the webview's identity — and `webview_log` does the rest.
///
/// `async` so a hot error path runs on the runtime, not the main thread.
#[tauri::command]
pub async fn log_webview(
    state: State<'_, AppState>,
    webview: Webview,
    record: WebviewRecord,
) -> Result<()> {
    // Each window holds one webview under the same label, so the window is
    // what resolves it.
    webview_log::record_from(&state.webview_log, webview.label(), record, |label| {
        webview.get_webview_window(label).is_some()
    });
    Ok(())
}

/// Forwards a GraphQL query/mutation to the sidecar over UDS HTTP. Returns
/// body + real HTTP status; only a transport failure is `Err` (frontend
/// `networkError`/retry). Shape matches `src/lib/graphql/invoke-fetch.ts`.
#[tauri::command]
pub async fn graphql_query(state: State<'_, AppState>, body: String) -> Result<GraphqlResponse> {
    state.sidecar.query(body).await
}

/// Registers a GraphQL subscription (one SSE connection to the sidecar).
/// Returns the op id for [`graphql_unsubscribe`]. Shape matches
/// `src/lib/graphql/subscribe-exchange.ts`.
///
/// Tagged with the calling webview's label so the host can tear the
/// subscription down when that page goes away — a reload or a window close
/// runs no JS teardown (see `lib.rs`).
#[tauri::command]
pub async fn graphql_subscribe(
    state: State<'_, AppState>,
    webview: Webview,
    query: String,
    variables: serde_json::Value,
    channel: Channel<String>,
) -> Result<u64> {
    state
        .sidecar
        .subscribe(query, variables, channel, webview.label().to_string())
        .await
}

/// Cancels a subscription. Tolerant of unknown ids — the frontend can race
/// teardown with the subscribe resolve (subscribe-exchange.ts).
#[tauri::command]
pub async fn graphql_unsubscribe(state: State<'_, AppState>, id: u64) -> Result<()> {
    state.sidecar.unsubscribe(id).await;
    Ok(())
}
