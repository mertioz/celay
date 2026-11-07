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
    ("windows", "amd64"): "Celay-Windows-x64.zip",
    ("windows", "arm64"): "Celay-Windows-ARM64.zip",
    ("darwin", "arm64"): "Celay-macOS-Apple-Silicon.zip",
    ("darwin", "amd64"): "Celay-macOS-Intel.zip",
    ("linux", "amd64"): "Celay-Linux-x86_64.tar.gz",
    ("linux", "arm64"): "Celay-Linux-ARM64.tar.gz",
}


def build_portable(go, source, output, osname, arch):
    flags = "-s -w -H=windowsgui" if osname == "windows" else "-s -w"
    subprocess.run([
        go, "build", "-p", "4", "-mod=readonly", "-trimpath", "-ldflags=" + flags,
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
        "CFBundleShortVersionString": "2.0",
        "CFBundleVersion": "2",
        "LSMinimumSystemVersion": "12.0",
        "LSUIElement": True,
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
    binaries.mkdir(exist_ok=True)
    license_file = project / "LICENSE"

    def binary_for(osname, arch):
        suffix = ".exe" if osname == "windows" else ""
        return binaries / f"celay-{osname}-{arch}{suffix}"

    if not args.package_only:
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            futures = [pool.submit(build_portable, args.go, portable, binary_for(osname, arch), osname, arch)
                       for osname, arch in TARGETS]
            for future in futures:
                future.result()

    notices = licenses(args.go, portable)
    packagers = {"windows": windows_archive, "darwin": mac_archive, "linux": linux_archive}
    for (osname, arch), filename in TARGETS.items():
        packagers[osname](versions / filename, binary_for(osname, arch), license_file, notices)
    with zipfile.ZipFile(versions / "Celay-Windows-macOS-Linux.zip", "w") as archive:
        archive.write(project / "README.md", "README.md", compress_type=zipfile.ZIP_DEFLATED)
        archive.write(license_file, "LICENSE", compress_type=zipfile.ZIP_DEFLATED)
        for filename in TARGETS.values():
            archive.write(versions / filename, filename)


if __name__ == "__main__":
    main()
