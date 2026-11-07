import argparse
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import tarfile
import tempfile
import zipfile


TARGETS = {
    ("windows", "amd64"): "Celay-Windows-x64.zip",
    ("windows", "arm64"): "Celay-Windows-ARM64.zip",
    ("darwin", "arm64"): "Celay-macOS-Apple-Silicon.zip",
    ("darwin", "amd64"): "Celay-macOS-Intel.zip",
    ("linux", "amd64"): "Celay-Linux-x86_64.tar.gz",
    ("linux", "arm64"): "Celay-Linux-ARM64.tar.gz",
}


def build_native(go, source, output, osname, arch):
    flags = "-s -w -H=windowsgui" if osname == "windows" else "-s -w"
    subprocess.run([
        go, "build", "-p", "4", "-mod=readonly", "-trimpath", "-ldflags=" + flags,
        "-o", str(output), ".",
    ], cwd=source, env={**os.environ, "CGO_ENABLED": "1", "GOOS": osname, "GOARCH": arch}, check=True)


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


def windows_archive(output, binary, license_file, notices):
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.write(binary, "Celay.exe")
        archive.write(license_file, "LICENSE")
        archive.writestr("Licences.txt", notices)


def mac_archive(output, binary, license_file, notices):
    app = "Celay.app/Contents"
    info = plistlib.dumps({
        "CFBundleExecutable": "Celay",
        "CFBundleIdentifier": "local.celay.app",
        "CFBundleName": "Celay",
        "CFBundleDisplayName": "Celay",
        "CFBundlePackageType": "APPL",
        "CFBundleShortVersionString": "2.1.1",
        "CFBundleVersion": "2.1.1",
        "LSMinimumSystemVersion": "12.0",
        "NSHighResolutionCapable": True,
    })
    with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
        archive.writestr(f"{app}/Info.plist", info)
        executable = zipfile.ZipInfo(f"{app}/MacOS/Celay")
        executable.create_system = 3
        executable.external_attr = 0o100755 << 16
        archive.writestr(executable, binary.read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
        archive.write(license_file, "LICENSE")
        archive.writestr("Licences.txt", notices)


def linux_archive(output, binary, license_file, notices):
    from io import BytesIO

    with tarfile.open(output, "w:gz", compresslevel=7) as archive:
        program = archive.gettarinfo(binary, "Celay/celay")
        program.mode = 0o755
        with binary.open("rb") as stream:
            archive.addfile(program, stream)
        archive.add(license_file, "Celay/LICENSE")
        license_info = tarfile.TarInfo("Celay/Licences.txt")
        license_info.size = len(notices)
        archive.addfile(license_info, BytesIO(notices))


def appimage_name(arch):
    return f"Celay-Linux-{'x86_64' if arch == 'amd64' else 'ARM64'}.AppImage"


def build_appimage(tool, output, binary, project, notices, arch):
    with tempfile.TemporaryDirectory() as temp:
        appdir = Path(temp) / "Celay.AppDir"
        program = appdir / "usr" / "bin" / "celay"
        program.parent.mkdir(parents=True)
        shutil.copy2(binary, program)
        program.chmod(0o755)
        shutil.copy2(project / "LICENSE", appdir / "LICENSE")
        (appdir / "Licences.txt").write_bytes(notices)
        shutil.copy2(project / "src" / "portable" / "celay.svg", appdir / "celay.svg")
        (appdir / "Celay.desktop").write_text(
            "[Desktop Entry]\nType=Application\nName=Celay\n"
            "Comment=Export et programmation des discussions Codex\n"
            "Exec=celay\nIcon=celay\nTerminal=false\nCategories=Utility;\n", encoding="utf-8")
        launcher = appdir / "AppRun"
        launcher.write_text('#!/bin/sh\nexec "$(dirname "$(readlink -f "$0")")/usr/bin/celay" "$@"\n')
        launcher.chmod(0o755)
        subprocess.run([
            str(tool.resolve()), "--no-appstream", str(appdir), str(output),
        ], env={**os.environ, "APPIMAGE_EXTRACT_AND_RUN": "1",
                "ARCH": "x86_64" if arch == "amd64" else "aarch64"}, check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--go", default="go")
    parser.add_argument("--binaries", type=Path)
    parser.add_argument("--package-only", action="store_true")
    parser.add_argument("--target", nargs=2, metavar=("GOOS", "GOARCH"))
    parser.add_argument("--build-only", action="store_true")
    parser.add_argument("--appimage-tool", type=Path)
    args = parser.parse_args()
    project = Path(__file__).resolve().parent
    versions = project / "versions"
    binaries = args.binaries.resolve() if args.binaries else project / ".build"
    portable = project / "src" / "portable"
    versions.mkdir(exist_ok=True)
    binaries.mkdir(exist_ok=True)
    license_file = project / "LICENSE"

    def binary_for(osname, arch):
        suffix = ".exe" if osname == "windows" else ""
        return binaries / f"celay-{osname}-{arch}{suffix}"

    if not args.package_only:
        target = tuple(args.target or subprocess.check_output(
            [args.go, "env", "GOOS", "GOARCH"], cwd=portable, text=True).split())
        if target not in TARGETS:
            parser.error("Target not supported.")
        build_native(args.go, portable, binary_for(*target), *target)
        if args.appimage_tool:
            if target[0] != "linux":
                parser.error("AppImage can only be built for Linux.")
            build_appimage(args.appimage_tool, binaries / appimage_name(target[1]),
                           binary_for(*target), project, licenses(args.go, portable), target[1])
        if args.build_only:
            return
        targets = [target]
    else:
        targets = list(TARGETS)

    notices = licenses(args.go, portable)
    packagers = {"windows": windows_archive, "darwin": mac_archive, "linux": linux_archive}
    for osname, arch in targets:
        filename = TARGETS[osname, arch]
        packagers[osname](versions / filename, binary_for(osname, arch), license_file, notices)
        if osname == "linux" and (image := binaries / appimage_name(arch)).is_file():
            shutil.copy2(image, versions / image.name)
    if not args.package_only:
        return
    with zipfile.ZipFile(versions / "Celay-Windows-macOS-Linux.zip", "w") as archive:
        archive.write(project / "README.md", "README.md", compress_type=zipfile.ZIP_DEFLATED)
        archive.write(license_file, "LICENSE", compress_type=zipfile.ZIP_DEFLATED)
        for (osname, arch), filename in TARGETS.items():
            path = versions / filename
            if osname == "linux" and (image := versions / appimage_name(arch)).is_file():
                with image.open("rb") as stream:
                    entry = zipfile.ZipInfo(image.name)
                    entry.create_system = 3
                    entry.external_attr = 0o100755 << 16
                    with archive.open(entry, "w") as dest:
                        shutil.copyfileobj(stream, dest)
            else:
                archive.write(path, path.name)


if __name__ == "__main__":
    main()
