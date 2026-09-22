import io
import os
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
MANAGER = REPO_ROOT / "script/deploy/manage.sh"


def make_package(directory: Path, version: str, marker: str) -> Path:
    payload = directory / f"dwportal-{version}"
    payload.mkdir()
    binary = payload / "dwportal"
    binary.write_text(f"#!/bin/sh\necho {marker}\n", encoding="utf-8")
    binary.chmod(0o755)
    (payload / "dwportal.service").write_text(
        "[Service]\nUser=dwportal\nGroup=dwportal\nProtectHome=read-only\n",
        encoding="utf-8",
    )
    (payload / "VERSION").write_text(version + "\n", encoding="utf-8")
    archive = directory / f"dwportal-{version}-linux-amd64.tar.gz"
    with tarfile.open(archive, "w:gz") as bundle:
        bundle.add(payload, arcname=payload.name)
    return archive


class DeploymentLifecycleTests(unittest.TestCase):
    def test_install_upgrade_sync_and_uninstall(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary = Path(temporary_directory)
            root = temporary / "root"
            test_config = temporary / "config.yaml"
            test_config.write_text(
                "schema_version: 1\nbase_domain: dw.c\nservices:\n"
                "  - id: old\n    name: Old\n    domain: old.dw.c\n    port: 80\n",
                encoding="utf-8",
            )
            environment = os.environ | {
                "DWPORTAL_ROOT_PREFIX": str(root),
                "DWPORTAL_SKIP_SYSTEMD": "1",
                "DWPORTAL_TEST_CONFIG": str(test_config),
            }
            first = make_package(temporary, "0.1.0", "first")
            second = make_package(temporary, "0.2.0", "second")

            subprocess.run([MANAGER, "install", "--package", first], env=environment, check=True)
            current = root / "opt/dwportal/current"
            self.assertEqual(current.resolve().name, "0.1.0")
            catalog = root / "var/lib/dwportal/catalog.yaml"
            self.assertEqual(catalog.resolve(), test_config.resolve())
            unit = root / "etc/systemd/system/dwportal.service"
            self.assertIn("User=", unit.read_text(encoding="utf-8"))
            self.assertIn("ProtectHome=read-only", unit.read_text(encoding="utf-8"))

            subprocess.run([MANAGER, "upgrade", "--package", second], env=environment, check=True)
            self.assertEqual(current.resolve().name, "0.2.0")

            caddyfile = temporary / "Caddyfile"
            caddyfile.write_text("git.dw.c {\n reverse_proxy 127.0.0.1:3000\n}\n", encoding="utf-8")
            subprocess.run([MANAGER, "sync-config", "--caddyfile", caddyfile], env=environment, check=True)
            self.assertIn('id: "git"', test_config.read_text(encoding="utf-8"))

            subprocess.run([MANAGER, "uninstall"], env=environment, check=True)
            self.assertFalse((root / "opt/dwportal").exists())
            self.assertTrue((root / "var/lib/dwportal").exists())
            subprocess.run([MANAGER, "uninstall", "--purge"], env=environment, check=True)
            self.assertFalse((root / "var/lib/dwportal").exists())

    def test_failed_upgrade_rolls_back_current_link(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            temporary = Path(temporary_directory)
            root = temporary / "root"
            test_config = temporary / "config.yaml"
            test_config.write_text(
                "schema_version: 1\nbase_domain: dw.c\nservices:\n"
                "  - id: one\n    name: One\n    domain: one.dw.c\n    port: 80\n",
                encoding="utf-8",
            )
            bin_directory = temporary / "bin"
            bin_directory.mkdir()
            fake_systemctl = bin_directory / "systemctl"
            fake_systemctl.write_text(
                "#!/bin/sh\n"
                "if [ \"${FAIL_RESTART:-0}\" = 1 ] && [ \"$1\" = restart ]; then exit 1; fi\n"
                "exit 0\n",
                encoding="utf-8",
            )
            fake_systemctl.chmod(0o755)
            environment = os.environ | {
                "PATH": str(bin_directory) + os.pathsep + os.environ["PATH"],
                "DWPORTAL_ROOT_PREFIX": str(root),
                "DWPORTAL_TEST_CONFIG": str(test_config),
            }
            first = make_package(temporary, "1.0.0", "first")
            second = make_package(temporary, "2.0.0", "second")
            subprocess.run([MANAGER, "install", "--package", first], env=environment, check=True)
            failed_environment = environment | {"FAIL_RESTART": "1"}
            result = subprocess.run(
                [MANAGER, "upgrade", "--package", second], env=failed_environment, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual((root / "opt/dwportal/current").resolve().name, "1.0.0")


if __name__ == "__main__":
    unittest.main()
