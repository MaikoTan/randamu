# Windows: installer and scheduled task

The release archives include a per-architecture installer
(`randamu_<version>_windows_<arch>_setup.exe`). It installs to
`%ProgramFiles%\randamu`, adds the directory to the machine `PATH`, bundles
tesseract on x64/x86, registers an uninstaller and needs administrator
rights. It does not create a scheduled task; use the scripts below for that.

`install.ps1` registers a per-user scheduled task ("randamu") that runs
`randamu.exe` from this directory once a day; no administrator rights are
needed. `uninstall.ps1` removes it again.

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1   # defaults: 09:00 + 1h delay
```

Parameters: `-ConfigPath` (default `%APPDATA%\randamu\config.json`), `-At`
(daily start time), `-RandomDelay` (default `01:00:00`). The script does not
create the config file and does not set the refresh token; both are manual
steps, see the main README.
