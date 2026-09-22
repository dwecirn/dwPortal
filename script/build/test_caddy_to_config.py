import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("caddy_to_config.py")
SPEC = importlib.util.spec_from_file_location("caddy_to_config", MODULE_PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class CaddyToConfigTests(unittest.TestCase):
    def test_parses_subdomains_and_ignores_unrelated_sites(self):
        services = MODULE.parse_caddyfile(
            """
            git.dw.c {
                reverse_proxy 127.0.0.1:3000
            }
            chat.dw.c {
                encode gzip
                reverse_proxy http://localhost:8008
            }
            example.com {
                reverse_proxy localhost:9000
            }
            """,
            "dw.c",
        )
        self.assertEqual(
            [(item.service_id, item.domain, item.port) for item in services],
            [("chat", "chat.dw.c", 8008), ("git", "git.dw.c", 3000)],
        )

    def test_atomic_output_is_valid_portal_yaml_shape(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "config.yaml"
            service = MODULE.Service("git", "Git", "git.dw.c", 3000)
            MODULE.atomic_write(output, MODULE.render_yaml("dw.c", [service]))
            self.assertEqual(
                output.read_text(encoding="utf-8"),
                'schema_version: 1\nbase_domain: "dw.c"\nlisten_port: 6480\nservices:\n'
                '  - id: "git"\n'
                '    name: "Git"\n'
                '    domain: "git.dw.c"\n'
                "    port: 3000\n",
            )


if __name__ == "__main__":
    unittest.main()
