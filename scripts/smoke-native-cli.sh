#!/usr/bin/env bash

set -euo pipefail

binary_path="${1:?binary path is required}"
expected_version="${2:-}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
smoke_root="${repo_root}/.native-cli-smoke"
config_root="${smoke_root}/config"

cleanup() {
  rm -rf "${smoke_root}"
}
trap cleanup EXIT

rm -rf "${smoke_root}"
mkdir -p "${config_root}"

version_json="${smoke_root}/version.json"
schema_json="${smoke_root}/schema.json"
doctor_json="${smoke_root}/doctor.json"

"${binary_path}" --version > "${version_json}"
"${binary_path}" schema > "${schema_json}"
doctor_config_unavailable=false
if [[ "$(uname -s)" == "Darwin" ]]; then
  # Go ignores XDG_CONFIG_HOME on macOS. Without HOME, doctor checks the
  # missing configuration directory without reading the user's credentials.
  env -u HOME "${binary_path}" doctor > "${doctor_json}"
  doctor_config_unavailable=true
else
  # Go uses APPDATA on Windows and XDG_CONFIG_HOME on other Unix systems.
  APPDATA="${config_root}" XDG_CONFIG_HOME="${config_root}" "${binary_path}" doctor > "${doctor_json}"
fi

python3 - "${version_json}" "${schema_json}" "${doctor_json}" "${expected_version}" "${doctor_config_unavailable}" <<'PY'
import json
import sys
from pathlib import Path

version_path, schema_path, doctor_path, expected_version, config_unavailable = sys.argv[1:6]
version = json.loads(Path(version_path).read_text())
schema = json.loads(Path(schema_path).read_text())
doctor = json.loads(Path(doctor_path).read_text())
expected_commands = [
    "auth.login",
    "auth.logout",
    "auth.status",
    "blasts.send",
    "cohosts.invite",
    "cohosts.link.create",
    "cohosts.link.revoke",
    "cohosts.remove",
    "cohosts.revoke-invite",
    "contacts.list",
    "doctor",
    "events.cancel",
    "events.create",
    "events.get",
    "events.list",
    "events.update",
    "guests.invite",
    "guests.list",
    "posters.list",
    "posters.search",
    "rsvp.get",
    "rsvp.set",
    "schema",
    "version",
]
if version.get("ok") is not True:
    raise SystemExit("--version did not return success")
if version["meta"]["command"] != "version":
    raise SystemExit("--version reported the wrong command")
if version["data"]["version"] != version["meta"]["cliVersion"]:
    raise SystemExit("--version data/meta version mismatch")
if expected_version and version["data"]["version"] != expected_version:
    raise SystemExit(f"--version={version['data']['version']!r}, want {expected_version!r}")
for key in ("productContractRevision", "remoteContractRevision"):
    if not version["data"].get(key):
        raise SystemExit(f"--version missing {key}")
if schema.get("ok") is not True or schema["meta"]["command"] != "schema":
    raise SystemExit("schema did not return success")
if schema["data"].get("commands") != expected_commands:
    raise SystemExit("schema command catalog drifted")
if doctor.get("ok") is not True or doctor["meta"]["command"] != "doctor":
    raise SystemExit("doctor did not return success")
checks = doctor["data"].get("checks", [])
if doctor["data"].get("healthy") is not False:
    raise SystemExit("doctor should report unhealthy without credentials")
expected_check = {
    "name": "credentials",
    "status": "fail",
    "message": "Authentication credentials are missing.",
    "remediation": "Establish authentication before using commands that require it.",
}
if config_unavailable == "true":
    expected_check["message"] = "Configuration directory is unavailable."
    expected_check["remediation"] = "Set a usable user configuration directory."
if checks != [expected_check]:
    raise SystemExit("doctor output drifted")
PY
