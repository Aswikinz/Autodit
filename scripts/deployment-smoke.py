"""Verify clean deployment from an offline bundle in an isolated Podman namespace.

Creates fresh secrets, database and snapshots; never touches the live stack.
Requires the host Podman/Compose prerequisites and a free loopback port.
"""
import argparse
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tarfile
import tempfile
import time
import urllib.request
import uuid
from safe_paths import confined_path

root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--bundle", type=Path, default=root / "dist/autodit-0.1.0-offline.tar.gz", help="Archive under dist/; relative paths start at the repository root")
parser.add_argument("--port", type=int, default=8089)
parser.add_argument("--local", action="store_true", help="Verify administrator login and local accounts")
parser.add_argument("--browser", action="store_true", help="Run browser acceptance tests for the selected authentication mode")
parser.add_argument("--use-local-images", action="store_true", help="Use repository deployment files and already built images")
args = parser.parse_args()
try:
    bundle = confined_path(root / 'dist', root / args.bundle)
except ValueError:
    parser.error('--bundle must remain inside dist/')
if not args.use_local_images and (not bundle.name.endswith('.tar.gz') or not bundle.is_file()):
    parser.error('--bundle must name an existing .tar.gz archive inside dist/')
podman = shutil.which("podman") or r"C:\Program Files\RedHat\Podman\podman.exe"
project = "autodit_smoke_" + uuid.uuid4().hex[:10]
origin = f"http://localhost:{args.port}"
environment = {**os.environ, "PODMAN_COMPOSE_PROVIDER": "podman-compose"}

def run(*command, **kwargs):
    return subprocess.run([podman, *command], check=True, env=environment, **kwargs)

with tempfile.TemporaryDirectory(prefix="autodit-deployment-") as temporary:
    workspace = Path(temporary)
    if args.use_local_images:
        stage = workspace / "autodit"
        shutil.copytree(root / "deploy", stage / "deploy")
    else:
        with tarfile.open(bundle) as archive:
            archive.extractall(workspace, filter="data")
        stage = workspace / "autodit"
        for line in (stage / "checksums.txt").read_text().splitlines():
            expected, name = line.split("  ", 1)
            file = (stage / name).resolve()
            if not file.is_relative_to(stage.resolve()):
                raise RuntimeError("Bundle checksum path escapes directory")
            with file.open("rb") as source:
                if hashlib.file_digest(source, "sha256").hexdigest() != expected:
                    raise RuntimeError("Bundle checksum mismatch")
        for image in sorted((stage / "dist/images").glob("*.tar")):
            run("load", "-i", str(image), stdout=subprocess.DEVNULL)

    # Only resource names, port and public origin differ from the verified bundle.
    compose_path = stage / "deploy/compose/compose.json"
    configuration = json.loads(compose_path.read_text())
    configuration["name"] = project
    secret_names = {name: name.replace("autodit_", project + "_", 1) for name in configuration["secrets"]}
    configuration["secrets"] = {name: {"external": True} for name in secret_names.values()}
    for service in configuration["services"].values():
        service["pull_policy"] = "never"
        for secret in service.get("secrets", []):
            secret["source"] = secret_names[secret["source"]]
    compose_path.write_text(json.dumps(configuration, indent=2))
    (stage / ".env").write_text(f"AUTODIT_VERSION=0.1.0\nAUTODIT_PORT={args.port}\nAUTODIT_PUBLIC_URL={origin}\nAUTODIT_AUTH_MODE={'local' if args.local else 'demo'}\n")
    (stage / "inbox").mkdir(mode=0o755)
    secret_dir = stage / "secrets"
    secret_dir.mkdir(mode=0o700)
    compose = ["compose", "--env-file", str(stage / ".env"), "-f", str(compose_path), "-p", project]
    created_secrets = []
    try:
        for original, target in secret_names.items():
            value = secrets.token_urlsafe(36)
            file = secret_dir / original
            file.write_text(value)
            file.chmod(0o600)
            run("secret", "create", target, str(file), stdout=subprocess.DEVNULL)
            created_secrets.append(target)
        run(*compose, "up", "-d", "postgres")
        run(*compose, "--profile", "setup", "run", "--rm", "-T", "migrate")
        run(*compose, "--profile", "setup", "run", "--rm", "-T", "bootstrap")
        run(*compose, "up", "-d", "api", "worker", "proxy")
        client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        csrf = ""

        def request(path, body=None, method=None):
            data = None if body is None else json.dumps(body).encode()
            req = urllib.request.Request(origin + path, data=data, headers={"Content-Type": "application/json", "Origin": origin, "X-CSRF-Token": csrf}, method=method)
            with client.open(req, timeout=5) as response:
                return {} if response.status == 204 else json.load(response)

        for attempt in range(60):
            try:
                if request("/healthz")["status"] == "ok":
                    break
            except (OSError, ValueError):
                pass
            time.sleep(1)
        else:
            raise RuntimeError("Fresh deployment did not become healthy")
        browser_password = secrets.token_urlsafe(24)
        if args.local:
            initial = (secret_dir / "autodit_admin_password").read_text()
            request("/api/password-login", {"username": "admin", "password": initial})
            session = request("/api/session")
            assert session["identity"]["must_change_password"]
            csrf = session["identity"]["csrf_token"]
            request("/api/password", {"current_password": initial, "new_password": browser_password})
            request("/api/password-login", {"username": "admin", "password": browser_password})
            csrf = request("/api/session")["identity"]["csrf_token"]
            admin = request("/api/admin/users")["users"][0]
            admin["roles"] = ["admin", "audit_manager", "auditor", "implementer", "rule_engineer"]
            request("/api/admin/users", admin)
            request("/api/password-login", {"username": "admin", "password": browser_password})
            csrf = request("/api/session")["identity"]["csrf_token"]
            policy = request("/api/parameters")
            assert policy["parameters"]["explicit_currencies"] and not policy["parameters"].get("currency_thresholds")
            policy["parameters"]["currency_thresholds"] = {"USD": "1000.0000"}
            request("/api/parameters", {"parameters": policy["parameters"], "revision": policy["revision"]}, "PUT")
        else:
            request("/api/login", {"token": (secret_dir / "autodit_demo_token").read_text()})
            csrf = request("/api/session")["identity"]["csrf_token"]
        population = json.loads((root / "test/fixtures/population.json").read_text())
        request("/api/sources", {"id": population["source_id"], "name": "Synthetic deployment verification", "interval_minutes": 60, "mapping": {}})
        run_id = request("/api/runs", population)["id"]
        for attempt in range(60):
            result = request("/api/runs/" + run_id)
            status = result.get("run", result).get("status")
            if status == "completed":
                break
            if status == "failed":
                raise RuntimeError("Fresh deployment audit failed")
            time.sleep(1)
        else:
            raise RuntimeError("Fresh deployment audit timed out")
        queue = request("/api/exceptions")
        if queue["total"] != 4:
            raise RuntimeError("Fresh deployment findings differ from the golden population")
        detail = request("/api/exceptions/" + queue["items"][0]["key"])
        replay = request("/api/observations/" + detail["observations"][0]["id"] + "/replay", {})
        if replay.get("result") != detail["observations"][0]["result"]:
            raise RuntimeError("Fresh deployment evidence replay failed")
        partial = stage / "inbox/synthetic.partial"
        partial.write_text(json.dumps(population))
        partial.chmod(0o644)  # Fictional fixture readable by the non-root worker.
        time.sleep(3)
        if len(request("/api/runs")) != 1:
            raise RuntimeError("Worker consumed an incomplete file")
        partial.rename(stage / "inbox/synthetic.json")
        for attempt in range(60):
            runs = request("/api/runs")
            if len(runs) == 2 and all(item["status"] == "completed" for item in runs):
                break
            time.sleep(1)
        else:
            raise RuntimeError("Automatic file ingestion did not complete")
        run("restart", project + "_worker_1", stdout=subprocess.DEVNULL)
        time.sleep(4)
        if len(request("/api/runs")) != 2 or request("/api/exceptions")["total"] != 4:
            raise RuntimeError("Inbox receipt did not survive a worker restart")
        if args.browser:
            test_env = {**os.environ, "AUTODIT_TEST_URL": origin, "AUTODIT_TEST_DB_PASSWORD": (secret_dir / "autodit_app_password").read_text()}
            if args.local:
                test_env["AUTODIT_TEST_PASSWORD"] = browser_password
                test_env.pop("AUTODIT_TEST_TOKEN", None)
            else:
                test_env["AUTODIT_TEST_TOKEN"] = (secret_dir / "autodit_demo_token").read_text()
                test_env.pop("AUTODIT_TEST_PASSWORD", None)
            npm = shutil.which("npm.cmd") or shutil.which("npm")
            browser_tests = [npm, "run", "test:e2e"]
            if not args.local:
                browser_tests += ["--", "workspace.spec.ts"]
            subprocess.run(browser_tests, cwd=root / "web", env=test_env, check=True)
        print("Clean-deployment smoke passed: fresh database, native secrets, golden findings, verified replay and automatic inbox restart deduplication.")
    finally:
        subprocess.run([podman, *compose, "down", "-v"], env=environment, check=False)
        for name in created_secrets:
            subprocess.run([podman, "secret", "rm", name], check=False, stdout=subprocess.DEVNULL)
