; S2shenDPI NSIS Installer Hooks

!macro NSIS_HOOK_PREINSTALL
    nsExec::ExecToStack 'taskkill /F /IM S2shenDPI.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM s2shen-proxy.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM s2shen-divert.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM "s2shen-divert-x86_64-pc-windows-msvc.exe"'
    Pop $0
    Sleep 1000

    nsExec::ExecToStack 'sc.exe stop WinDivert'
    Pop $0
    nsExec::ExecToStack 'sc.exe stop WinDivert14'
    Pop $0
    Sleep 1500
    nsExec::ExecToStack 'sc.exe delete WinDivert'
    Pop $0
    nsExec::ExecToStack 'sc.exe delete WinDivert14'
    Pop $0
    Sleep 1000

    Delete "$LOCALAPPDATA\S2shenDPI\WinDivert64.sys"
    Delete "$LOCALAPPDATA\S2shenDPI\WinDivert.dll"

    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyEnable" 0
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyServer"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyOverride"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "AutoConfigURL"

    nsExec::ExecToStack 'netsh winhttp reset proxy'
    Pop $0

    Delete "$TEMP\s2shendpi_proxy_active.lock"
    Delete "$TEMP\s2shendpi_sidecar.pid"
    Delete "$TEMP\s2shendpi_divert.pid"
    Delete "$TEMP\s2shendpi_divert.log"
!macroend

!macro NSIS_HOOK_PREUNINSTALL
    nsExec::ExecToStack 'taskkill /F /IM S2shenDPI.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM s2shen-proxy.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM s2shen-divert.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM "s2shen-divert-x86_64-pc-windows-msvc.exe"'
    Pop $0
    Sleep 1000

    nsExec::ExecToStack 'sc.exe stop WinDivert'
    Pop $0
    nsExec::ExecToStack 'sc.exe stop WinDivert14'
    Pop $0
    Sleep 1500
    nsExec::ExecToStack 'sc.exe delete WinDivert'
    Pop $0
    nsExec::ExecToStack 'sc.exe delete WinDivert14'
    Pop $0
    Sleep 1000

    Delete "$LOCALAPPDATA\S2shenDPI\WinDivert64.sys"
    Delete "$LOCALAPPDATA\S2shenDPI\WinDivert.dll"

    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyEnable" 0
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyServer"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyOverride"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "AutoConfigURL"

    nsExec::ExecToStack 'netsh winhttp reset proxy'
    Pop $0

    Delete "$TEMP\s2shendpi_proxy_active.lock"
    Delete "$TEMP\s2shendpi_sidecar.pid"
    Delete "$TEMP\s2shendpi_divert.pid"
    Delete "$TEMP\s2shendpi_divert.log"

    nsExec::ExecToStack 'netsh advfirewall firewall delete rule name=S2shenDPI_Proxy'
    Pop $0
    nsExec::ExecToStack 'netsh advfirewall firewall delete rule name=S2shenDPI_PAC'
    Pop $0

    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "S2shenDPI"

    nsExec::ExecToStack 'ipconfig /flushdns'
    Pop $0
!macroend
