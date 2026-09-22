#!/usr/bin/env python3
"""Generate dwPortal's config.yaml from a Caddyfile without modifying Caddy."""

from __future__ import annotations

import argparse
import ipaddress
import json
import os
import re
import tempfile
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlsplit


@dataclass(frozen=True)
class Service:
    service_id: str
    name: str
    domain: str
    port: int


def strip_comment(line: str) -> str:
    quoted = False
    escaped = False
    output: list[str] = []
    for char in line:
        if char == '"' and not escaped:
            quoted = not quoted
        if char == "#" and not quoted:
            break
        output.append(char)
        escaped = char == "\\" and not escaped
        if char != "\\":
            escaped = False
    return "".join(output).strip()


def normalize_domain(address: str) -> str | None:
    address = address.strip().strip(",")
    if not address or address.startswith(":"):
        return None
    candidate = address if "://" in address else f"//{address}"
    parsed = urlsplit(candidate)
    host = parsed.hostname
    if not host:
        return None
    try:
        ipaddress.ip_address(host)
        return None
    except ValueError:
        return host.rstrip(".").lower()


def upstream_port(upstream: str) -> int | None:
    upstream = upstream.strip().strip(",")
    if upstream.startswith(("unix/", "unix//")) or upstream.startswith("{"):
        return None
    parsed = urlsplit(upstream if "://" in upstream else f"//{upstream}")
    try:
        return parsed.port
    except ValueError:
        return None


def display_name(service_id: str) -> str:
    return " ".join(part.capitalize() for part in re.split(r"[-_]+", service_id))


def parse_caddyfile(text: str, base_domain: str) -> list[Service]:
    services: dict[str, Service] = {}
    active_domains: list[str] = []
    depth = 0

    for line_number, raw_line in enumerate(text.splitlines(), start=1):
        line = strip_comment(raw_line)
        if not line:
            continue

        opens = line.count("{")
        closes = line.count("}")
        if depth == 0 and opens:
            header = line.split("{", 1)[0].strip()
            active_domains = [
                domain
                for token in header.split()
                if (domain := normalize_domain(token)) is not None
            ]

        if active_domains and re.match(r"^reverse_proxy(?:\s|$)", line):
            parts = line.split()
            upstream = next(
                (part for part in parts[1:] if not part.startswith(("@", "{"))),
                "",
            )
            port = upstream_port(upstream)
            if port is None:
                raise ValueError(
                    f"line {line_number}: reverse_proxy needs an explicit TCP port"
                )
            for domain in active_domains:
                if domain == base_domain or not domain.endswith("." + base_domain):
                    continue
                service_id = domain[: -(len(base_domain) + 1)].replace(".", "-")
                services[service_id] = Service(
                    service_id=service_id,
                    name=display_name(service_id),
                    domain=domain,
                    port=port,
                )

        depth += opens - closes
        if depth <= 0:
            depth = 0
            active_domains = []

    return sorted(services.values(), key=lambda item: item.service_id)


def render_yaml(base_domain: str, services: list[Service], listen_port: int = 6480) -> str:
    lines = [
        "schema_version: 1",
        f"base_domain: {json.dumps(base_domain)}",
        f"listen_port: {listen_port}",
        "services:",
    ]
    for service in services:
        lines.extend(
            [
                f"  - id: {json.dumps(service.service_id)}",
                f"    name: {json.dumps(service.name)}",
                f"    domain: {json.dumps(service.domain)}",
                f"    port: {service.port}",
            ]
        )
    return "\n".join(lines) + "\n"


def atomic_write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{path.name}.", suffix=".tmp", dir=path.parent, text=True
    )
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as temporary:
            temporary.write(content)
            temporary.flush()
            os.fsync(temporary.fileno())
        os.replace(temporary_name, path)
    except BaseException:
        Path(temporary_name).unlink(missing_ok=True)
        raise


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True, type=Path, help="Caddyfile to read")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("config/catalog.yaml"),
        help="generated catalog to write (default: config/catalog.yaml)",
    )
    parser.add_argument(
        "--base-domain", default="dw.c", help="only include subdomains of this domain"
    )
    parser.add_argument(
        "--listen-port", type=int, default=6480, help="portal HTTP port (default: 6480)"
    )
    args = parser.parse_args()

    base_domain = args.base_domain.strip().strip(".").lower()
    if not 1 <= args.listen_port <= 65535:
        parser.error("--listen-port must be between 1 and 65535")
    services = parse_caddyfile(args.input.read_text(encoding="utf-8"), base_domain)
    if not services:
        parser.error(f"no reverse-proxied *.{base_domain} sites found in {args.input}")
    atomic_write(args.output, render_yaml(base_domain, services, args.listen_port))
    print(f"generated {args.output} with {len(services)} service(s)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
