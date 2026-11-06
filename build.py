import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import plistlib
import subprocess
import tarfile
import zipfile


TARGETS = {
    ("darwin", "arm64"): "macOS-Apple-Silicon.zip",
    ("darwin", "amd64"): "macOS-Intel.zip",
    ("linux", "amd64"): "Linux-x86_64.tar.gz",
    ("linux", "arm64"): "Linux-ARM64.tar.gz",
}


def build_native(source, output):
    framework = Path(os.environ["WINDIR"]) / "Microsoft.NET"
    compiler = next(path for family in ("Framework64", "Framework")
                    if (path := framework / family / "v4.0.30319" / "csc.exe").is_file())
    subprocess.run([
        str(compiler), "/nologo", "/codepage:65001", "/target:winexe", "/platform:anycpu", "/optimize+",
        "/reference:System.Windows.Forms.dll", "/reference:System.Drawing.dll",
        "/reference:System.Web.Extensions.dll", f"/out:{output}", str(source),
    ], check=True)


def build_portable(go, source, output, osname, arch):
    subprocess.run([
        go, "build", "-p", "4", "-mod=readonly", "-trimpath", "-ldflags=-s -w",
        "-o", str(output), ".",
    ], cwd=source, env={**os.environ, "CGO_ENABLED": "0", "GOOS": osname, "GOARCH": arch}, check=True)


def licenses(go, source):
    goroot = subprocess.check_output([go, "env", "GOROOT"], cwd=source, text=True).strip()
    modules = subprocess.check_output([go, "list", "-m", "-json", "all"], cwd=source, text=True)
    notices = [("Go", Path(goroot))]
    decoder = json.JSONDecoder()
    while modules.strip():
        module, end = decoder.raw_decode(modules.lstrip())
        modules = modules.lstrip()[end:]
        if "Dir" in module and not module.get("Main"):
            notices.append((module["Path"], Path(module["Dir"])))
    sections = []
    for name, folder in notices:
        for notice in sorted(folder.iterdir()):
            if notice.is_file() and notice.name.upper().startswith(("LICENSE", "COPYING", "NOTICE")):
                sections.append(f"=== {name} / {notice.name} ===\n\n{notice.read_text(encoding='utf-8', errors='replace')}")
    return "\n\n".join(sections).encode()


def mac_archive(output, binary, license_file, notices):
    app = "Codex Markdown.app/Contents"
    info = plistlib.dumps({
        "CFBundleExecutable": "CodexMarkdown",
        "CFBundleIdentifier": "local.codexmarkdown.app",
        "CFBundleName": "Codex Markdown",
        "CFBundleDisplayName": "Codex Markdown",
        "CFBundlePackageType": "APPL",
        "CFBundleShortVersionString": "1.0",
        "CFBundleVersion": "1",
        "LSMinimumSystemVersion": "12.0",
        "LSUIElement": True,
    })
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr(f"{app}/Info.plist", info)
        executable = zipfile.ZipInfo(f"{app}/MacOS/CodexMarkdown")
        executable.create_system = 3
        executable.external_attr = 0o100755 << 16
        archive.writestr(executable, binary.read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
        archive.write(license_file, "LICENSE")
        archive.writestr("Licences.txt", notices)


def linux_archive(output, binary, license_file, notices):
    from io import BytesIO

    with tarfile.open(output, "w:gz", compresslevel=7) as archive:
        program = archive.gettarinfo(binary, "Codex-Markdown/codex-markdown")
        program.mode = 0o755
        with binary.open("rb") as stream:
            archive.addfile(program, stream)
        archive.add(license_file, "Codex-Markdown/LICENSE")
        license_info = tarfile.TarInfo("Codex-Markdown/Licences.txt")
        license_info.size = len(notices)
        archive.addfile(license_info, BytesIO(notices))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--go", default="go")
    parser.add_argument("--binaries", type=Path)
    parser.add_argument("--package-only", action="store_true")
    args = parser.parse_args()
    project = Path(__file__).resolve().parent
    versions = project / "versions"
    binaries = args.binaries.resolve() if args.binaries else project / ".build"
    portable = project / "src" / "portable"
    versions.mkdir(exist_ok=True)
    windows = versions / "Windows"
    windows.mkdir(exist_ok=True)
    binaries.mkdir(exist_ok=True)
    windows_exe = windows / "Codex-Markdown.exe"
    license_file = project / "LICENSE"

    def binary_for(osname, arch):
        return binaries / f"codex-markdown-{osname}-{arch}"

    if not args.package_only:
        build_native(project / "src" / "windows" / "CodexMarkdown.cs", windows_exe)
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            futures = [pool.submit(build_portable, args.go, portable, binary_for(osname, arch), osname, arch)
                       for osname, arch in TARGETS]
            for future in futures:
                future.result()

    with zipfile.ZipFile(versions / "Codex-Markdown-Windows.zip", "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(windows_exe, "Codex-Markdown.exe")
        archive.write(license_file, "LICENSE")

    notices = licenses(args.go, portable)
    for (osname, arch), filename in TARGETS.items():
        package = mac_archive if osname == "darwin" else linux_archive
        package(versions / filename, binary_for(osname, arch), license_file, notices)
    with zipfile.ZipFile(versions / "Codex-Markdown-Windows-macOS-Linux.zip", "w") as archive:
        archive.write(project / "README.md", "README.md", compress_type=zipfile.ZIP_DEFLATED)
        archive.write(license_file, "LICENSE", compress_type=zipfile.ZIP_DEFLATED)
        archive.write(windows_exe, "Windows/Codex-Markdown.exe", compress_type=zipfile.ZIP_DEFLATED)
        for filename in TARGETS.values():
            archive.write(versions / filename, filename)


if __name__ == "__main__":
    main()
