# Windows: scheduled task

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
