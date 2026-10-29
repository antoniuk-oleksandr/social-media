#!/usr/bin/env python3

import json
import os
import subprocess
import sys
import time
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
        ["docker", "rm", *sorted(created)],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(
            f"warning: could not remove local Lambda containers: {result.stderr.strip()}",
            file=sys.stderr,
        )


def wait_for_log_collection() -> None:
    try:
        seconds = float(os.environ.get("LOCAL_LOG_COLLECTION_GRACE_SECONDS", "15"))
    except ValueError:
        seconds = 15
    if seconds > 0:
        time.sleep(seconds)


def save_application_logs(output: str, function_name: str) -> None:
    log_lines = []
    service_name = function_name.removesuffix("Function").lower()
    for line in output.splitlines():
        stripped = line.strip()
        if stripped.startswith('{"level":') and '"msg":' in stripped:
            try:
                record = json.loads(stripped)
            except json.JSONDecodeError:
                continue
            record["service_name"] = service_name
            log_lines.append(json.dumps(record, separators=(",", ":")))
    if not log_lines:
        return
    log_path = Path(os.environ.get("LOCAL_LOG_PATH", "/tmp/social-media-local-logs/sam-lambda.jsonl"))
    log_path.parent.mkdir(parents=True, exist_ok=True)
    with log_path.open("a", encoding="utf-8") as stream:
        stream.write("\n".join(log_lines) + "\n")


def load_dotenv(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        if key:
            values[key] = value
    return values


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

    requested_function = os.environ.get("SAM_FUNCTION")
    function_name = requested_function if requested_function in {"AuthFunction", "UsersFunction", "PostsFunction", "AdsFunction", "FeedFunction"} else "AuthFunction"
    env_path = Path(__file__).with_name(".env")
    env_file = Path(__file__).with_name("env.json")
    env_values = json.loads(env_file.read_text(encoding="utf-8"))
    env_values.setdefault(function_name, {}).update(load_dotenv(env_path))
    event_path = Path(__file__).with_name(".invoke-event.json")
    runtime_env_path = Path(__file__).with_name(".runtime-env.json")
    event_path.write_text(json.dumps(event), encoding="utf-8")
    runtime_env_path.write_text(json.dumps(env_values), encoding="utf-8")
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
                str(runtime_env_path),
                "--docker-network",
                "social-media-local",
            ],
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        print(result.stdout, end="")
        save_application_logs(result.stdout, function_name)
        return result.returncode
    finally:
        event_path.unlink(missing_ok=True)
        runtime_env_path.unlink(missing_ok=True)
        wait_for_log_collection()
        remove_created_containers(containers_before)


if __name__ == "__main__":
    raise SystemExit(main())
