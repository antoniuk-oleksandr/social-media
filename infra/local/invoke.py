#!/usr/bin/env python3

import json
import os
import subprocess
import sys
from pathlib import Path


def docker_container_ids() -> set[str]:
    result = subprocess.run(
        ["docker", "ps", "-aq"],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(
            f"warning: could not list Docker containers: {result.stderr.strip()}",
            file=sys.stderr,
        )
        return set()
    return {line for line in result.stdout.splitlines() if line}


def remove_created_containers(before: set[str]) -> None:
    created = docker_container_ids() - before
    if not created:
        return

    result = subprocess.run(
        ["docker", "rm", "--force", *sorted(created)],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(
            f"warning: could not remove local Lambda containers: {result.stderr.strip()}",
            file=sys.stderr,
        )


def main() -> int:
    if len(sys.argv) != 5:
        print(
            "usage: invoke.py METHOD PATH BODY HEADERS_JSON",
            file=sys.stderr,
        )
        return 2

    method = sys.argv[1].upper()
    path = sys.argv[2]
    body = sys.argv[3]
    headers = json.loads(sys.argv[4])
    if not isinstance(headers, dict):
        raise ValueError("headers must be a JSON object")

    event = {
        "version": "2.0",
        "routeKey": f"{method} {path}",
        "rawPath": path,
        "headers": headers,
        "requestContext": {
            "requestId": "local-request",
            "http": {
                "method": method,
                "path": path,
                "protocol": "HTTP/1.1",
                "sourceIp": "127.0.0.1",
                "userAgent": "sam-local",
            },
        },
        "body": body,
        "isBase64Encoded": False,
    }

    function_name = "UsersFunction" if os.environ.get("SAM_FUNCTION") == "UsersFunction" else "AuthFunction"
    event_path = Path(__file__).with_name(".invoke-event.json")
    event_path.write_text(json.dumps(event), encoding="utf-8")
    containers_before = docker_container_ids()
    try:
        result = subprocess.run(
            [
                "sam",
                "local",
                "invoke",
                function_name,
                "--template-file",
                "infra/local/template.yaml",
                "--event",
                str(event_path),
                "--env-vars",
                "infra/local/env.json",
                "--docker-network",
                "social-media-local",
            ],
            check=False,
        )
        return result.returncode
    finally:
        event_path.unlink(missing_ok=True)
        remove_created_containers(containers_before)


if __name__ == "__main__":
    raise SystemExit(main())
