import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch, Mock

spec = importlib.util.spec_from_file_location("service", Path(__file__).resolve().parents[1] / "ops" / "readonly_catalog_service.py")
service = importlib.util.module_from_spec(spec)
spec.loader.exec_module(service)


class ReadonlyServiceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.private = self.root / "private"
        self.private.mkdir(mode=0o700)
        (self.root / "bin").mkdir()
        self.binary = self.root / "bin" / "readonly"
        self.binary.write_bytes(b"test-only-binary")
        self.binary.chmod(0o700)
        self.digest = hashlib.sha256(self.binary.read_bytes()).hexdigest()
        self.write("product.url", "postgres://test:SECRET@127.0.0.1:55440/panta_mvp_preview")
        self.write("catalog.url", "postgres://test:SECRET@127.0.0.1:55433/indexcore_panta_mvp_catalog_20261014")
        self.config = {"database_url_file": str(self.private / "product.url"), "listen_address": "127.0.0.1:8180", "origin": "http://127.0.0.1:8180", "indexcore_base_url": "http://127.0.0.1:48892", "bindings": [{"binding_id": str(n), "label": "scope"} for n in range(3)]}
        self.write("product.json", json.dumps(self.config))
        self.receipt = {"version": 1, "product_binary": str(self.binary), "product_sha256": self.digest, "product_config": str(self.private / "product.json"), "catalog_binary": str(self.binary), "catalog_sha256": self.digest, "catalog_database_url_file": str(self.private / "catalog.url"), "catalog_postgres_id": "a"*64, "product_postgres_id": "b"*64}
        self.write("receipt.json", json.dumps(self.receipt))

    def write(self, name, value):
        path = self.private / name
        path.write_text(value)
        path.chmod(0o600)

    def load(self):
        return service.load_receipt(self.private / "receipt.json")

    def test_valid_receipt_and_binary_pin(self):
        result = self.load()
        self.assertEqual(result[2], self.binary)
        self.binary.write_bytes(b"changed")
        with self.assertRaises(service.Refused): self.load()

    def test_reject_permissions_symlink_duplicate_and_unknown_fields(self):
        path = self.private / "receipt.json"
        path.chmod(0o644)
        with self.assertRaises(service.Refused): self.load()
        path.chmod(0o600)
        self.write("receipt.json", '{"version":1,"version":2}')
        with self.assertRaises(service.Refused): self.load()
        self.receipt["worker_enabled"] = True
        self.write("receipt.json", json.dumps(self.receipt))
        with self.assertRaises(service.Refused): self.load()
        link = self.root / "bin" / "link"
        link.symlink_to(self.binary)
        with self.assertRaises(service.Refused): service.verified_binary(self.root, link, self.digest)

    def test_refuse_public_origin_or_wrong_database(self):
        self.config["listen_address"] = "0.0.0.0:8180"
        self.write("product.json", json.dumps(self.config))
        with self.assertRaises(service.Refused): self.load()
        self.config["listen_address"] = "127.0.0.1:8180"
        self.write("product.json", json.dumps(self.config))
        self.write("catalog.url", "postgres://test@127.0.0.1:55433/indexcore_gate318")
        with self.assertRaises(service.Refused): self.load()
        for override in ("host=other.invalid", "port=5432", "dbname=indexcore_gate318", "sslmode=disable&sslmode=require"):
            self.write("catalog.url", "postgres://test@127.0.0.1:55433/indexcore_panta_mvp_catalog_20261014?"+override)
            with self.assertRaises(service.Refused): self.load()
        with self.assertRaises(service.Refused): service.verified_binary(self.root, self.private / "product.json", self.digest)

    def test_never_inherit_provider_worker_or_hint_credentials(self):
        with patch.dict(os.environ, {"OPENLIST_PASSWORD": "SECRET", "PANTA_115_COOKIE": "SECRET", "PANTA_ACQUISITION_WORKER_ENABLED": "true", "INDEXCORE_HINT_TOKEN": "SECRET"}):
            env = service.child_environment("product", "/private/product.json", "PRIVATE_DSN")
            self.assertEqual(env["PANTA_ACQUISITION_WORKER_ENABLED"], "false")
            self.assertNotIn("OPENLIST_PASSWORD", env)
            self.assertNotIn("PANTA_115_COOKIE", env)
            env = service.child_environment("catalog", "/private/product.json", "PRIVATE_DSN")
            self.assertEqual(env["INDEXCORE_DATABASE_URL"], "PRIVATE_DSN")
            self.assertNotIn("INDEXCORE_HINT_TOKEN", env)

    def test_container_identity_failure_never_starts_any_container(self):
        with patch.object(service.subprocess, "run", return_value=Mock(stdout="wrong|exited|mvp-ui-preview-20261014\n", returncode=0)) as run:
            with self.assertRaises(service.Refused): service.ensure_database("product", "b"*64)
            self.assertEqual(run.call_count, 1)

    def test_start_only_pinned_owned_database_not_restart(self):
        replies = [Mock(stdout="b"*64+"|exited|mvp-ui-preview-20261014\n", returncode=0), Mock(returncode=0), Mock(returncode=0)]
        with patch.object(service.subprocess, "run", side_effect=replies) as run:
            service.ensure_database("product", "b"*64)
            self.assertEqual(run.call_args_list[1].args[0], ["docker", "start", "b"*64])
            self.assertFalse(any("restart" in call.args[0] for call in run.call_args_list))

    def test_controller_not_ready_is_bounded_and_never_creates_container(self):
        with patch.object(service.subprocess, "run", return_value=Mock(returncode=1)) as run, patch.object(service.time, "monotonic", side_effect=[0, 41]):
            with self.assertRaises(service.Refused): service.ensure_database("catalog", "a"*64)
            self.assertEqual(run.call_count, 1)
            self.assertEqual(run.call_args.args[0][1], "inspect")

    def test_unsupported_action_never_executes(self):
        with patch.object(service.os, "execve") as execute:
            for mode in ("scan", "migrate", "worker", "start-acquisition", "delete"):
                with self.assertRaises(service.Refused): service.main([mode, str(self.private / "receipt.json")])
            execute.assert_not_called()


if __name__ == "__main__": unittest.main()
