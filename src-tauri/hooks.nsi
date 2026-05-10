; DarknesDPI NSIS Installer Hooks

!macro NSIS_HOOK_PREINSTALL
    nsExec::ExecToStack 'taskkill /F /IM DarknesDPI.exe'
    Pop $0
    nsExec::ExecToStack 'taskkill /F /IM darknes-proxy.exe'
    Pop $0
    Sleep 500

    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyEnable" 0
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyServer"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyOverride"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "AutoConfigURL"

    nsExec::ExecToStack 'netsh winhttp reset proxy'
    Pop $0

    Delete "$TEMP\darknesdpi_proxy_active.lock"
    Delete "$TEMP\darknesdpi_sidecar.pid"
!macroend

!macro NSIS_HOOK_PREUNINSTALL
    nsExec::ExecToStack 'taskkill /F /IM DarknesDPI.exe'
    Pop $0
    Sleep 1000

    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyEnable" 0
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyServer"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyOverride"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "AutoConfigURL"

    nsExec::ExecToStack 'netsh winhttp reset proxy'
    Pop $0

    Delete "$TEMP\darknesdpi_proxy_active.lock"
    Delete "$TEMP\darknesdpi_sidecar.pid"

    nsExec::ExecToStack 'taskkill /F /IM darknes-proxy.exe'
    Pop $0

    nsExec::ExecToStack 'netsh advfirewall firewall delete rule name=DarknesDPI_Proxy'
    Pop $0
    nsExec::ExecToStack 'netsh advfirewall firewall delete rule name=DarknesDPI_PAC'
    Pop $0

    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "DarknesDPI"

    nsExec::ExecToStack 'ipconfig /flushdns'
    Pop $0
!macroend
