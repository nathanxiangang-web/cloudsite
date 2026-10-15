"""Owned readonly process launcher. Not a scanner, downloader or DB migrator."""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import time
import urllib.parse
import urllib.request


class Refused(Exception):
    pass


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise Refused("duplicate configuration")
        result[key] = value
    return result


def protected(path, maximum=16384):
    path = Path(path)
    info = path.lstat()
    parent = path.parent.stat()
    if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
            or info.st_mode & 0o077 or parent.st_uid != os.getuid()
            or stat.S_IMODE(parent.st_mode) != 0o700 or info.st_size > maximum):
        raise Refused("unprotected configuration")
    return path.read_text(encoding="utf-8")


def inside(root, value, directory):
    path = Path(value)
    if not path.is_absolute() or path.is_symlink():
        raise Refused("path rejected")
    path = path.resolve()
    if not path.is_relative_to(root / directory):
        raise Refused("path outside owned deployment")
    return path


def verified_binary(root, name, checksum):
    if not isinstance(checksum, str) or not re.fullmatch(r"[0-9a-f]{64}", checksum):
        raise Refused("binary receipt rejected")
    path = inside(root, name, "bin")
    info = path.stat()
    if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid()
            or info.st_mode & 0o022 or not info.st_mode & stat.S_IXUSR
            or hashlib.sha256(path.read_bytes()).hexdigest() != checksum):
        raise Refused("binary identity rejected")
    return path


def load_receipt(receipt_path):
    path = Path(receipt_path)
    receipt = json.loads(protected(path), object_pairs_hook=unique_object)
    expected = {"version", "product_binary", "product_sha256", "product_config",
                "catalog_binary", "catalog_sha256", "catalog_database_url_file",
                "catalog_postgres_id", "product_postgres_id"}
    if set(receipt) != expected or receipt["version"] != 1 or path.parent.name != "private":
        raise Refused("service receipt rejected")
    for key in ("catalog_postgres_id", "product_postgres_id"):
        if not re.fullmatch(r"[0-9a-f]{64}", receipt[key]):
            raise Refused("database container receipt rejected")
    root = path.parent.parent.resolve()
    product = verified_binary(root, receipt["product_binary"], receipt["product_sha256"])
    catalog = verified_binary(root, receipt["catalog_binary"], receipt["catalog_sha256"])
    config_path = inside(root, receipt["product_config"], "private")
    config = json.loads(protected(config_path), object_pairs_hook=unique_object)
    legacy = {"database_url_file", "listen_address", "origin", "indexcore_base_url", "bindings"}
    if (set(config) not in (legacy, legacy | {"file_access_config_file"}, legacy | {"file_access_config_file", "sharing_enabled"})
            or "sharing_enabled" in config and not isinstance(config["sharing_enabled"], bool)
            or config["listen_address"] != "127.0.0.1:8180"
            or config["origin"] != "http://127.0.0.1:8180"
            or config["indexcore_base_url"] != "http://127.0.0.1:48892"
            or not isinstance(config["bindings"], list) or len(config["bindings"]) != 3):
        raise Refused("readonly product scope rejected")
    if "file_access_config_file" in config:
        access = json.loads(protected(inside(root, config["file_access_config_file"], "private")), object_pairs_hook=unique_object)
        if set(access) != {"base_url", "token_file", "cache_directory"} or access["base_url"] != "http://127.0.0.1:5244":
            raise Refused("file content endpoint rejected")
        token = protected(inside(root, access["token_file"], "private"), 8192).strip()
        if not token or "\r" in token or "\n" in token:
            raise Refused("file content credential rejected")
        cache = inside(root, access["cache_directory"], "private")
        info = cache.stat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
            raise Refused("preview cache directory rejected")
    product_dsn = protected(inside(root, config["database_url_file"], "private"), 4096).strip()
    catalog_dsn = protected(inside(root, receipt["catalog_database_url_file"], "private"), 4096).strip()
    for dsn, port, database in [(product_dsn, 55440, "panta_mvp_preview"),
                               (catalog_dsn, 55433, "indexcore_panta_mvp_catalog_20261014")]:
        parsed = urllib.parse.urlparse(dsn)
        # libpq/pgx accept host/port/dbname options in the query string. Reject
        # such overrides instead of checking only the URL authority/path.
        options = urllib.parse.parse_qsl(parsed.query, strict_parsing=True, keep_blank_values=True)
        if (parsed.scheme not in ("postgres", "postgresql") or parsed.hostname != "127.0.0.1"
                or parsed.port != port or parsed.path != "/" + database or parsed.fragment
                or len(options) > 1 or any(key != "sslmode" for key, _ in options)):
            raise Refused("database scope rejected")
    return root, receipt, product, catalog, config_path, catalog_dsn


def ensure_database(mode, pinned):
    name = "p12-postgres" if mode == "catalog" else "panta-mvp-ui-preview-20261014"
    template = '{{.Id}}|{{.State.Status}}|{{index .Config.Labels "panta.owner"}}'
    # User services may start before the system Docker daemon after boot. Wait
    # boundedly for inspect, never create/recreate or retarget a container.
    end = time.monotonic() + 40
    while True:
        result = subprocess.run(["docker", "inspect", "--format", template, name],
                                capture_output=True, text=True, timeout=5)
        if result.returncode == 0:
            break
        if time.monotonic() >= end:
            raise Refused("database controller unavailable")
        time.sleep(0.5)
    identity, state, owner = result.stdout.strip().split("|")
    if (identity != pinned or state not in ("running", "exited")
            or mode == "product" and owner != "mvp-ui-preview-20261014"):
        raise Refused("database container identity rejected")
    if state == "exited":
        subprocess.run(["docker", "start", identity], capture_output=True, timeout=25, check=True)
    end = time.monotonic() + 40
    while time.monotonic() < end:
        ready = subprocess.run(["docker", "exec", identity, "pg_isready"], capture_output=True, timeout=4)
        if ready.returncode == 0:
            return
        time.sleep(0.5)
    raise Refused("database readiness timeout")


def child_environment(mode, config_path, catalog_dsn):
    env = {key: os.environ[key] for key in ("PATH", "HOME", "USER", "LANG", "LC_ALL") if key in os.environ}
    if mode == "product":
        env.update(PANTA_PRODUCT_CONFIG_FILE=str(config_path), PANTA_ACQUISITION_WORKER_ENABLED="false")
    elif mode == "catalog":
        env["INDEXCORE_DATABASE_URL"] = catalog_dsn
    else:
        raise Refused("unknown readonly service mode")
    return env


def main(args):
    if len(args) != 2 or args[0] not in ("catalog", "product"):
        raise Refused("readonly service invocation rejected")
    mode, receipt_path = args
    root, receipt, product, catalog, config_path, dsn = load_receipt(receipt_path)
    ensure_database(mode, receipt[mode + "_postgres_id"])
    if mode == "product":
        end = time.monotonic() + 60
        while True:
            try:
                with urllib.request.urlopen("http://127.0.0.1:48892/readyz", timeout=2) as response:
                    if response.status == 200:
                        break
            except Exception:
                pass
            if time.monotonic() >= end:
                raise Refused("readonly catalog not ready")
            time.sleep(0.5)
    binary = product if mode == "product" else catalog
    command = [str(binary)] + (["serve-product-readonly"] if mode == "product" else [])
    os.chdir(root)
    os.umask(0o077)
    os.execve(binary, command, child_environment(mode, config_path, dsn))


if __name__ == "__main__":
    try:
        main(sys.argv[1:])
    except Exception:
        # Never echo exceptions containing a private DSN, source, or raw body.
        sys.stderr.write("readonly service startup refused\n")
        sys.exit(1)
