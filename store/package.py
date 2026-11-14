import argparse
import json
from pathlib import Path
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET


FOUNDATION = "http://schemas.microsoft.com/appx/manifest/foundation/windows10"
UAP = "http://schemas.microsoft.com/appx/manifest/uap/windows10"
UAP10 = "http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
RESCAP = "http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
DESKTOP6 = "http://schemas.microsoft.com/appx/manifest/desktop/windows10/6"
VIRTUALIZATION = "http://schemas.microsoft.com/appx/manifest/virtualization/windows10"


def manifest(identity, arch, version):
    for prefix, uri in [("", FOUNDATION), ("uap", UAP), ("uap10", UAP10),
                        ("rescap", RESCAP), ("desktop6", DESKTOP6), ("virtualization", VIRTUALIZATION)]:
        ET.register_namespace(prefix, uri)

    def element(parent, name, attributes=None, text=None, namespace=FOUNDATION):
        item = ET.SubElement(parent, f"{{{namespace}}}{name}", attributes or {})
        item.text = text
        return item

    root = ET.Element(f"{{{FOUNDATION}}}Package", {"IgnorableNamespaces": "uap uap10 rescap desktop6 virtualization"})
    element(root, "Identity", {"Name": identity["name"], "Publisher": identity["publisher"],
                              "Version": version, "ProcessorArchitecture": arch})
    properties = element(root, "Properties")
    element(properties, "DisplayName", text="Celay")
    element(properties, "PublisherDisplayName", text=identity["publisher_display_name"])
    element(properties, "Description", text="Export et programmation des discussions Codex")
    element(properties, "Logo", text="Assets/StoreLogo.png")
    # Keep a single queue and instance lock when users move between portable
    # Celay and the Store edition. Windows 11 scopes the exclusion to Celay.
    element(properties, "FileSystemWriteVirtualization", text="disabled", namespace=DESKTOP6)
    virtual = element(properties, "FileSystemWriteVirtualization", namespace=VIRTUALIZATION)
    excluded = element(virtual, "ExcludedDirectories", namespace=VIRTUALIZATION)
    element(excluded, "ExcludedDirectory", text="$(KnownFolder:RoamingAppData)\\Celay", namespace=VIRTUALIZATION)
    dependencies = element(root, "Dependencies")
    element(dependencies, "TargetDeviceFamily", {"Name": "Windows.Desktop",
            "MinVersion": "10.0.19041.0", "MaxVersionTested": "10.0.28000.0"})
    resources = element(root, "Resources")
    element(resources, "Resource", {"Language": "fr-fr"})
    applications = element(root, "Applications")
    app = element(applications, "Application", {"Id": "Celay", "Executable": "Celay.exe",
            f"{{{UAP10}}}RuntimeBehavior": "packagedClassicApp", f"{{{UAP10}}}TrustLevel": "mediumIL"})
    element(app, "VisualElements", {"DisplayName": "Celay",
            "Description": "Export et programmation des discussions Codex",
            "Square150x150Logo": "Assets/Square150x150Logo.png",
            "Square44x44Logo": "Assets/Square44x44Logo.png",
            "BackgroundColor": "transparent"}, namespace=UAP)
    capabilities = element(root, "Capabilities")
    element(capabilities, "Capability", {"Name": "runFullTrust"}, namespace=RESCAP)
    element(capabilities, "Capability", {"Name": "unvirtualizedResources"}, namespace=RESCAP)
    ET.indent(root)
    return ET.ElementTree(root)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--makeappx", required=True, type=Path)
    parser.add_argument("--binaries", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--version", default="2.2.2.0")
    parser.add_argument("--go", default="go")
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[1]
    sys.path.insert(0, str(project))
    from build import licenses

    notices = licenses(args.go, project / "src/portable")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    identity = json.loads((project / "store/identity.json").read_text(encoding="utf-8"))
    for arch, goarch in [("x64", "amd64"), ("arm64", "arm64")]:
        stage = output / f"stage-{arch}"
        stage.mkdir(exist_ok=True)
        shutil.copy2(args.binaries / f"celay-windows-{goarch}.exe", stage / "Celay.exe")
        shutil.copy2(project / "LICENSE", stage / "LICENSE")
        (stage / "Licences.txt").write_bytes(notices)
        shutil.copytree(project / "store/assets", stage / "Assets", dirs_exist_ok=True)
        manifest(identity, arch, args.version).write(stage / "AppxManifest.xml", encoding="utf-8", xml_declaration=True)
        subprocess.run([str(args.makeappx), "pack", "/d", str(stage),
                        "/p", str(output / f"Celay_{args.version}_{arch}.msix"), "/o"], check=True)
    bundle_inputs = output / "bundle"
    bundle_inputs.mkdir(exist_ok=True)
    for arch in ("x64", "arm64"):
        shutil.copy2(output / f"Celay_{args.version}_{arch}.msix", bundle_inputs)
    subprocess.run([str(args.makeappx), "bundle", "/d", str(bundle_inputs),
                    "/p", str(output / f"Celay_{args.version}.msixbundle"), "/bv", args.version, "/o"], check=True)


if __name__ == "__main__":
    main()
