#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${1:-${repo_root}/dist}"
verify_root="${repo_root}/.native-release-verify"

cleanup() {
  rm -rf "${verify_root}"
}
trap cleanup EXIT

source_version="$(sed -n 's/^[[:space:]]*Version[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "${repo_root}/internal/app/buildinfo.go")"
if [[ -z "${source_version}" ]]; then
  echo "could not read source version" >&2
  exit 1
fi
snapshot_version="${SNAPSHOT_VERSION:-${source_version}-snapshot}"
checksum_file="${dist_dir}/partiful_${snapshot_version}_checksums.txt"

if ! command -v goreleaser >/dev/null 2>&1; then
  echo "goreleaser is required" >&2
  exit 1
fi
if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip is required" >&2
  exit 1
fi

rm -rf "${dist_dir}" "${verify_root}"
mkdir -p "${verify_root}"
(
  cd "${repo_root}"
  SNAPSHOT_VERSION="${snapshot_version}" goreleaser release --snapshot --clean
)

expected_archives=(
  "partiful_${snapshot_version}_darwin_amd64.tar.gz"
  "partiful_${snapshot_version}_darwin_arm64.tar.gz"
  "partiful_${snapshot_version}_linux_amd64.tar.gz"
  "partiful_${snapshot_version}_linux_arm64.tar.gz"
  "partiful_${snapshot_version}_windows_amd64.zip"
  "partiful_${snapshot_version}_windows_arm64.zip"
)

for archive in "${expected_archives[@]}"; do
  if [[ ! -f "${dist_dir}/${archive}" ]]; then
    echo "missing archive: ${archive}" >&2
    exit 1
  fi
done
if [[ ! -f "${checksum_file}" ]]; then
  echo "missing checksum file: $(basename "${checksum_file}")" >&2
  exit 1
fi

archive_count="$(python3 - "${checksum_file}" <<'PY'
import sys
from pathlib import Path
path = Path(sys.argv[1])
lines = [line for line in path.read_text().splitlines() if line.strip()]
print(len(lines))
PY
)"
if [[ "${archive_count}" != "6" ]]; then
  echo "checksum file should contain 6 entries, found ${archive_count}" >&2
  exit 1
fi

for archive in "${expected_archives[@]}"; do
  if ! grep -Fq " ${archive}" "${checksum_file}"; then
    echo "checksum file does not cover ${archive}" >&2
    exit 1
  fi
  base_name="${archive%.*}"
  if [[ "${archive}" == *.tar.gz ]]; then
    archive_root="${base_name%.tar}"
    binary_name="partiful"
    content="$(tar -tzf "${dist_dir}/${archive}")"
  else
    archive_root="${base_name}"
    binary_name="partiful.exe"
    content="$(unzip -Z1 "${dist_dir}/${archive}")"
  fi
  for required_path in \
    "${archive_root}/${binary_name}" \
    "${archive_root}/LICENSE" \
    "${archive_root}/README.md"; do
    if ! grep -Fxq "${required_path}" <<<"${content}"; then
      echo "archive ${archive} is missing ${required_path}" >&2
      exit 1
    fi
  done
done

(
  cd "${dist_dir}"
  sha256sum -c "$(basename "${checksum_file}")"
)

host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64) host_arch="amd64" ;;
  aarch64|arm64) host_arch="arm64" ;;
  *) host_arch="" ;;
esac
host_archive="${dist_dir}/partiful_${snapshot_version}_${host_os}_${host_arch}.tar.gz"
if [[ -n "${host_arch}" && -f "${host_archive}" ]]; then
  tar -xzf "${host_archive}" -C "${verify_root}"
  extracted_binary="${verify_root}/partiful_${snapshot_version}_${host_os}_${host_arch}/partiful"
  "${repo_root}/scripts/smoke-native-cli.sh" "${extracted_binary}" "${snapshot_version}"
fi
