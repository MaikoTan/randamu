#!/usr/bin/env python3
"""Prune an extracted tesseract install tree to what tesseract.exe loads.

The Windows installers ship ~160MB of unrelated tooling (GTK/ICU/Python
helpers, training binaries). This keeps tesseract.exe, the DLLs it imports
transitively and the data files, and deletes every other DLL. DLLs that do
not ship in the directory are Windows system libraries (KERNEL32, USER32,
...) and stay unbundled; an import that is missing but is not a known system
DLL makes the script fail, so a broken bundle never reaches a release.

Usage: prune_tesseract.py <extracted-dir> [--objdump PATH]
"""

import argparse
import os
import re
import subprocess
import sys

DLL_LINE = re.compile(r"DLL Name:\s*(\S+)")

# Windows system libraries that are never shipped with tesseract. Anything
# else that resolves to nothing is a real packaging bug.
SYSTEM_DLLS = {
    "advapi32.dll", "bcrypt.dll", "cfgmgr32.dll", "comctl32.dll",
    "comdlg32.dll", "crypt32.dll", "dnsapi.dll", "dwmapi.dll",
    "dwrite.dll", "gdi32.dll", "gdiplus.dll", "imm32.dll",
    "iphlpapi.dll", "kernel32.dll", "msimg32.dll", "msvcrt.dll",
    "mswsock.dll", "ncrypt.dll", "ntdll.dll", "ole32.dll",
    "oleaut32.dll", "powrprof.dll", "psapi.dll", "rpcrt4.dll",
    "secur32.dll", "setupapi.dll", "shell32.dll", "shlwapi.dll",
    "user32.dll", "userenv.dll", "usp10.dll", "uxtheme.dll",
    "version.dll", "winmm.dll", "wldap32.dll", "ws2_32.dll",
    "wtsapi32.dll",
}


def imports(objdump, path):
    """Return the DLL names the PE file at path imports."""
    out = subprocess.run(
        [objdump, "-p", path], capture_output=True, text=True, check=True
    ).stdout
    return DLL_LINE.findall(out)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("directory")
    ap.add_argument("--objdump", default="objdump")
    args = ap.parse_args()

    present = {e.lower(): e for e in os.listdir(args.directory)}
    if "tesseract.exe" not in present:
        sys.exit(f"tesseract.exe not found in {args.directory}")

    keep, queue, missing = set(), ["tesseract.exe"], set()
    while queue:
        name = queue.pop()
        key = name.lower()
        if key in keep:
            continue
        keep.add(key)
        for dep in imports(args.objdump, os.path.join(args.directory, present[key])):
            dkey = dep.lower()
            if dkey in present:
                queue.append(dkey)
            elif dkey not in SYSTEM_DLLS and not dkey.startswith("api-ms-"):
                missing.add(dep)

    if missing:
        sys.exit(f"unresolved imports: {', '.join(sorted(missing))}")

    removed = [e for e in os.listdir(args.directory)
               if e.lower().endswith(".dll") and e.lower() not in keep]
    for name in removed:
        os.remove(os.path.join(args.directory, name))
    print(f"kept {len(keep)} PE files, removed {len(removed)} unused DLLs")


if __name__ == "__main__":
    main()
