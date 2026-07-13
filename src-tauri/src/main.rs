// Prevents additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    std::panic::set_hook(Box::new(|panic_info| {
        #[cfg(target_os = "windows")]
        {
            use std::os::windows::process::CommandExt;
            const CREATE_NO_WINDOW: u32 = 0x08000000;

            let _ = std::process::Command::new("reg")
                .args([
                    "add",
                    "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
                    "/v",
                    "ProxyEnable",
                    "/t",
                    "REG_DWORD",
                    "/d",
                    "0",
                    "/f",
                ])
                .creation_flags(CREATE_NO_WINDOW)
                .status();

            let _ = std::process::Command::new("reg")
                .args([
                    "add",
                    "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings",
                    "/v",
                    "ProxyServer",
                    "/t",
                    "REG_SZ",
                    "/d",
                    "",
                    "/f",
                ])
                .creation_flags(CREATE_NO_WINDOW)
                .status();

            let _ = std::process::Command::new("taskkill")
                .args(["/F", "/IM", "s2shen-proxy.exe"])
                .creation_flags(CREATE_NO_WINDOW)
                .stdout(std::process::Stdio::null())
                .stderr(std::process::Stdio::null())
                .status();
        }

        #[cfg(target_os = "linux")]
        {
            // Proxy ayarlarını sıfırla
            let _ = std::process::Command::new("gsettings")
                .args(["set", "org.gnome.system.proxy", "mode", "none"])
                .status();
            // Süreçleri temizle
            let _ = std::process::Command::new("pkill")
                .args(["-f", "s2shen-proxy"])
                .status();
            let _ = std::process::Command::new("pkill")
                .args(["-f", "s2shen-divert"])
                .status();
            // PID lock temizle
            let _ = std::fs::remove_file("/tmp/s2shendpi.lock");
        }

        eprintln!("S2shenDPI PANIC: {}", panic_info);
    }));

    s2shen_tauri_lib::run()
}
