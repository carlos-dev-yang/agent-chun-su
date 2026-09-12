#!/usr/bin/env python3
"""Build private release candidates; never publish or include runtime data."""
import argparse
import datetime
import hashlib
import json
import os
import posixpath
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
from urllib.parse import urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64")


def run(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, text=True, **kwargs).strip()


def root_links(text, source):
    """Resolve links from a copied guide's source folder into the package root."""
    def replace(match):
        target = urlsplit(match.group(1))
        if target.scheme or target.netloc or not target.path or target.path.startswith("/"):
            return match.group(0)
        path = posixpath.normpath(posixpath.join(str(Path(source).parent), target.path))
        return "](" + urlunsplit(("", "", path, target.query, target.fragment)) + ")"
    return re.sub(r"\]\(([^)\s]+)\)", replace, text)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--target", action="append", choices=TARGETS, required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9._+-]+", args.version):
        parser.error("version must be a plain release identifier")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    revision = run("git", "rev-parse", "HEAD")
    dirty = bool(run("git", "status", "--porcelain"))
    modules = run("go", "list", "-m", "-json", "all")
    decoder, remaining, module_list = json.JSONDecoder(), modules, []
    while remaining.strip():
        module, consumed = decoder.raw_decode(remaining.lstrip())
        module_list.append(module)
        remaining = remaining.lstrip()[consumed:]
    public = run("git", "ls-files", "-z", "examples", "evaluation-skills",
                 "setup-skills", "workgroups", "docs", "*.md").split("\0")
    public = [path for path in public if path and not path.endswith(".go")]
    for target in dict.fromkeys(args.target):
        name = "chunsu-" + args.version + "-" + target.replace("/", "-")
        package = output / name
        if package.exists() or (output / (name + ".tar.gz")).exists():
            raise SystemExit("existing candidate preserved; select a new version or output directory")
        package.mkdir()
        goos, goarch = target.split("/")
        environment = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
        for binary in ("chunsu", "chunsu-secret-store"):
            subprocess.run(["go", "build", "-trimpath", "-ldflags", "-X main.version=" + args.version,
                            "-o", str(package / binary), "./cmd/" + binary], cwd=ROOT, env=environment, check=True)
        for source, dest in (("scripts/install.sh", "install.sh"),
                             ("docs/setup/SERVER.md", "SERVER.md"),
                             ("docs/contracts/credential-helper-v1.md", "CREDENTIALS.md")):
            shutil.copyfile(ROOT / source, package / dest)
        (package / "install.sh").chmod(0o755)
        guide = (package / "SERVER.md").read_text()
        (package / "SERVER.md").write_text(root_links(guide, "docs/setup/SERVER.md"))
        (package / "TARGET").write_text(target + "\n")
        for path in filter(None, public):
            dest = package / path
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / path, dest)
        notice = package / "notices"
        notice.mkdir()
        for module in module_list:
            if module.get("Main"):
                continue
            directory_name = module.get("Dir")
            if not directory_name:
                downloaded = json.loads(run("go", "mod", "download", "-json",
                                            module["Path"] + "@" + module["Version"]))
                directory_name = downloaded["Dir"]
            directory = Path(directory_name)
            licenses = [p for p in directory.iterdir() if p.is_file() and
                        p.name.upper().startswith(("LICENSE", "COPYING", "NOTICE"))]
            if not licenses:
                raise SystemExit("module license requires inspection: " + module["Path"])
            dest = notice / module["Path"].replace("/", "_")
            dest.mkdir()
            for license in licenses:
                shutil.copyfile(license, dest / license.name)
        manifest = {"version": args.version, "target": target, "source_revision": revision,
                    "working_tree_changes": dirty, "go_version": run("go", "version"),
                    "cgo_enabled": False, "built_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                    "modules": [{k: m[k] for k in ("Path", "Version", "Sum") if k in m} for m in module_list],
                    "publication": "local candidate only; unsigned"}
        (package / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        entries = []
        for path in sorted(package.rglob("*")):
            if path.is_file():
                entries.append(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + str(path.relative_to(package)))
        (package / "SHA256SUMS").write_text("\n".join(entries) + "\n")
        archive = output / (name + ".tar.gz")
        with tarfile.open(archive, "w:gz") as tar:
            tar.add(package, arcname=name)
        print(json.dumps({"archive": str(archive), "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                          "target": target}), flush=True)


if __name__ == "__main__":
    main()
