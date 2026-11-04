; randamu Windows installer.
;
; Compiled once per architecture from the Linux CI runner:
;   makensis -DVERSION=1.4.0 -DARCH=x64 -DPAYLOAD=dist/windows_x64 \
;            -DOUTFILE=dist/randamu_1.4.0_windows_x64_setup.exe installer.nsi
;
; The payload directory is the same staging tree that becomes the zip: it
; already contains randamu.exe, the docs and, for x64/x86, the bundled
; tesseract.
!include "MUI2.nsh"

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef ARCH
  !define ARCH "x64"
!endif
!ifndef PAYLOAD
  !error "PAYLOAD is required"
!endif
!ifndef OUTFILE
  !define OUTFILE "randamu-setup.exe"
!endif

Name "randamu ${VERSION}"
OutFile "${OUTFILE}"
Unicode true
RequestExecutionLevel admin

!if "${ARCH}" == "x86"
  InstallDir "$PROGRAMFILES32\randamu"
  InstallDirRegKey HKLM "Software\randamu" "InstallDir"
!else
  InstallDir "$PROGRAMFILES64\randamu"
  InstallDirRegKey HKLM "Software\randamu" "InstallDir"
!endif

!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\randamu"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "randamu" SEC_MAIN
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "${PAYLOAD}/randamu.exe"
  File "${PAYLOAD}/README.md"
  File "${PAYLOAD}/LICENSE"
  File "${PAYLOAD}/install.ps1"
  File "${PAYLOAD}/uninstall.ps1"
  ; x64/x86 carry tesseract; the arm64 build does not (no official build).
  !if /FileExists "${PAYLOAD}/THIRD-PARTY-NOTICES.md"
    File "${PAYLOAD}/THIRD-PARTY-NOTICES.md"
    File /r "${PAYLOAD}/tesseract"
  !endif

  WriteUninstaller "$INSTDIR/uninstall.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "randamu ${VERSION}"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "Maiko Tan"
  WriteRegStr HKLM "${UNINST_KEY}" "URLInfoAbout" "https://github.com/MaikoTan/randamu"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
  WriteRegStr HKLM "Software\randamu" "InstallDir" "$INSTDIR"

  ; Add the install directory to the machine PATH once. PowerShell is used
  ; because NSIS has no built-in environment editor; a missing PowerShell is
  ; tolerated and only costs the PATH entry.
  FileOpen $9 "$TEMP\randamu-path.ps1" w
  FileWrite $9 "$$p = [Environment]::GetEnvironmentVariable('Path','Machine'); if (-not $$p.Split(';').Contains('$INSTDIR')) { [Environment]::SetEnvironmentVariable('Path', $$p + ';$INSTDIR', 'Machine') }"
  FileClose $9
  nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -File "$TEMP\randamu-path.ps1"'
  Pop $0
  Delete "$TEMP\randamu-path.ps1"
SectionEnd

Section "Uninstall"
  ; Remove the PATH entry before deleting the directory so the script still
  ; has a stable path to compare against.
  FileOpen $9 "$TEMP\randamu-unpath.ps1" w
  FileWrite $9 "$$p = [Environment]::GetEnvironmentVariable('Path','Machine'); [Environment]::SetEnvironmentVariable('Path', (($$p.Split(';') | Where-Object { $$_ -ne '$INSTDIR' }) -join ';'), 'Machine')"
  FileClose $9
  nsExec::ExecToLog 'powershell -NoProfile -ExecutionPolicy Bypass -File "$TEMP\randamu-unpath.ps1"'
  Pop $0
  Delete "$TEMP\randamu-unpath.ps1"

  Delete "$INSTDIR\randamu.exe"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.md"
  Delete "$INSTDIR\install.ps1"
  Delete "$INSTDIR\uninstall.ps1"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$INSTDIR\tesseract"
  RMDir "$INSTDIR"
  DeleteRegKey HKLM "${UNINST_KEY}"
  DeleteRegKey HKLM "Software\randamu"
SectionEnd
