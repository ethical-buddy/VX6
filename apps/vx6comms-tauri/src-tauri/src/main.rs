#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::process::Command;

#[tauri::command]
fn vx6_status() -> Result<String, String> {
    run_vx6(&["status"])
}

#[tauri::command]
fn vx6_init(name: String) -> Result<String, String> {
    run_vx6(&["init", "--name", &name, "--listen", "[::]:4242"])
}

#[tauri::command]
fn vx6_node_start() -> Result<String, String> {
    // Starts foreground process; intended as bridge smoke command for now.
    run_vx6(&["node"])
}

fn run_vx6(args: &[&str]) -> Result<String, String> {
    let out = Command::new("vx6")
        .args(args)
        .output()
        .map_err(|e| format!("failed to run vx6: {e}"))?;
    let stdout = String::from_utf8_lossy(&out.stdout).to_string();
    let stderr = String::from_utf8_lossy(&out.stderr).to_string();
    if out.status.success() {
        Ok(stdout)
    } else {
        Err(format!("{stdout}\n{stderr}"))
    }
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![vx6_status, vx6_init, vx6_node_start])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

