#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git -C "${ROOT_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
RIPGREP_VERSION="${RIPGREP_VERSION:-15.1.0}"
RIPGREP_VENDOR_DIR="${RIPGREP_VENDOR_DIR:-${ROOT_DIR}/third_party/ripgrep}"
RIPGREP_DOWNLOAD_BASE_URL="${RIPGREP_DOWNLOAD_BASE_URL:-https://github.com/BurntSushi/ripgrep/releases/download/${RIPGREP_VERSION}}"
export COPYFILE_DISABLE=1
export COPY_EXTENDED_ATTRIBUTES_DISABLE=1

TARGETS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

assert_cli_bundle_self_contained() {
  local bundle="$1"
  local deps=(
    "ink"
    "react"
    "react/jsx-runtime"
    "ink-text-input"
  )

  for dep in "${deps[@]}"; do
    if grep -Fq "from \"$dep\"" "$bundle" || grep -Fq "from '$dep'" "$bundle"; then
      echo "CLI bundle still imports runtime dependency: $dep" >&2
      echo "Update cli/tsup.config.ts so release packages do not require cli/node_modules." >&2
      exit 1
    fi
  done

  if grep -Fq "packages/ink/node_modules/react" "$bundle"; then
    echo "CLI bundle contains a nested Ink React copy." >&2
    echo "Update cli/tsup.config.ts aliases so Ink and the app share one React instance." >&2
    exit 1
  fi

  local react_module_count
  react_module_count="$(grep -F '"node_modules/react/index.js"(exports, module)' "$bundle" | wc -l | tr -d '[:space:]')"
  if [[ "${react_module_count}" != "1" ]]; then
    echo "CLI bundle must contain exactly one React module marker; found ${react_module_count}." >&2
    echo "Duplicate React copies break Ink hooks at runtime." >&2
    exit 1
  fi
}

ripgrep_binary_name() {
  local goos="$1"
  if [[ "${goos}" == "windows" ]]; then
    echo "rg.exe"
  else
    echo "rg"
  fi
}

ripgrep_archive_name() {
  local goos="$1"
  local goarch="$2"
  case "${goos}/${goarch}" in
    darwin/amd64)
      echo "ripgrep-${RIPGREP_VERSION}-x86_64-apple-darwin.tar.gz"
      ;;
    darwin/arm64)
      echo "ripgrep-${RIPGREP_VERSION}-aarch64-apple-darwin.tar.gz"
      ;;
    linux/amd64)
      echo "ripgrep-${RIPGREP_VERSION}-x86_64-unknown-linux-musl.tar.gz"
      ;;
    linux/arm64)
      echo "ripgrep-${RIPGREP_VERSION}-aarch64-unknown-linux-gnu.tar.gz"
      ;;
    windows/amd64)
      echo "ripgrep-${RIPGREP_VERSION}-x86_64-pc-windows-msvc.zip"
      ;;
    *)
      echo "Unsupported ripgrep target: ${goos}/${goarch}" >&2
      exit 1
      ;;
  esac
}

download_ripgrep_vendor() {
  local goos="$1"
  local goarch="$2"
  local platform="${goos}-${goarch}"
  local rg_name
  rg_name="$(ripgrep_binary_name "${goos}")"
  local dest_dir="${RIPGREP_VENDOR_DIR}/${platform}"
  local dest="${dest_dir}/${rg_name}"
  local archive_name
  archive_name="$(ripgrep_archive_name "${goos}" "${goarch}")"
  local archive_url="${RIPGREP_DOWNLOAD_BASE_URL%/}/${archive_name}"
  local tmp_dir
  tmp_dir="$(mktemp -d)"

  echo "Downloading ripgrep ${RIPGREP_VERSION} for ${platform}..."
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 --connect-timeout 20 -o "${tmp_dir}/${archive_name}" "${archive_url}"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "${tmp_dir}/${archive_name}" "${archive_url}"
  else
    echo "Missing downloader: install curl or wget, or place ${rg_name} at ${dest} before packaging." >&2
    exit 1
  fi

  case "${archive_name}" in
    *.zip)
      if ! command -v unzip >/dev/null 2>&1; then
        echo "Missing unzip; install it or place ${rg_name} at ${dest} before packaging." >&2
        exit 1
      fi
      unzip -q "${tmp_dir}/${archive_name}" -d "${tmp_dir}/extract"
      ;;
    *.tar.gz)
      mkdir -p "${tmp_dir}/extract"
      tar -xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}/extract"
      ;;
    *)
      echo "Unsupported ripgrep archive: ${archive_name}" >&2
      exit 1
      ;;
  esac

  local extracted
  extracted="$(find "${tmp_dir}/extract" -type f -name "${rg_name}" -print -quit)"
  if [[ -z "${extracted}" ]]; then
    echo "Downloaded archive did not contain ${rg_name}: ${archive_url}" >&2
    exit 1
  fi

  mkdir -p "${dest_dir}"
  cp "${extracted}" "${dest}"
  if [[ "${goos}" != "windows" ]]; then
    chmod +x "${dest}"
  fi
  rm -rf "${tmp_dir}"
}

ensure_ripgrep_vendor() {
  local goos="$1"
  local goarch="$2"
  local platform="${goos}-${goarch}"
  local rg_name
  rg_name="$(ripgrep_binary_name "${goos}")"
  local source="${RIPGREP_VENDOR_DIR}/${platform}/${rg_name}"

  if [[ ! -f "${source}" ]]; then
    download_ripgrep_vendor "${goos}" "${goarch}"
  fi
  if [[ ! -f "${source}" ]]; then
    echo "Missing bundled ripgrep binary after download: ${source}" >&2
    exit 1
  fi
  if [[ "${goos}" != "windows" && ! -x "${source}" ]]; then
    chmod +x "${source}"
  fi
}

copy_ripgrep_vendor() {
  local goos="$1"
  local goarch="$2"
  local bin_dir="$3"
  local platform="${goos}-${goarch}"
  local rg_name
  rg_name="$(ripgrep_binary_name "${goos}")"
  ensure_ripgrep_vendor "${goos}" "${goarch}"
  local source="${RIPGREP_VENDOR_DIR}/${platform}/${rg_name}"
  local dest_dir="${bin_dir}/vendor/ripgrep/${platform}"

  mkdir -p "${dest_dir}"
  cp "${source}" "${dest_dir}/${rg_name}"
  if [[ "${goos}" != "windows" ]]; then
    chmod +x "${dest_dir}/${rg_name}"
  fi
}

cd "${ROOT_DIR}"

npm --prefix frontend run build
npm --prefix cli run build

if [[ ! -f cli/cli.cjs ]]; then
  echo "Missing CLI entry: cli/cli.cjs" >&2
  exit 1
fi
if [[ ! -f cli/dist/index.js ]]; then
  echo "Missing CLI bundle: cli/dist/index.js" >&2
  exit 1
fi
assert_cli_bundle_self_contained "cli/dist/index.js"

rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"
cp scripts/install.sh scripts/install.ps1 scripts/uninstall.sh scripts/uninstall.ps1 "${DIST_DIR}/"
chmod +x "${DIST_DIR}/install.sh" "${DIST_DIR}/uninstall.sh"

for target in "${TARGETS[@]}"; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  name="slimebot-${VERSION}-${goos}-${goarch}"
  package_dir="${DIST_DIR}/${name}"
  bin_dir="${package_dir}/bin"
  exe_suffix=""
  archive="${DIST_DIR}/${name}.tar.gz"
  if [[ "${goos}" == "windows" ]]; then
    exe_suffix=".exe"
    archive="${DIST_DIR}/${name}.zip"
  fi

  mkdir -p "${bin_dir}" "${package_dir}/cli"

  env GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X slimebot/internal/version.Version=${VERSION} -X slimebot/internal/version.Commit=${COMMIT} -X slimebot/internal/version.Date=${BUILD_DATE}" \
    -o "${bin_dir}/slimebot${exe_suffix}" ./cmd/server
  copy_ripgrep_vendor "${goos}" "${goarch}" "${bin_dir}"

  cp cli/cli.cjs "${package_dir}/cli/cli.cjs"
  cp -R cli/dist "${package_dir}/cli/dist"
  cp scripts/install.sh "${package_dir}/install.sh"
  cp scripts/install.ps1 "${package_dir}/install.ps1"
  cp scripts/uninstall.sh "${package_dir}/uninstall.sh"
  cp scripts/uninstall.ps1 "${package_dir}/uninstall.ps1"
  cp README.md README.en.md README.zh-CN.md LICENSE "${package_dir}/"
  if [[ -d docs ]]; then
    cp -R docs "${package_dir}/docs"
  fi
  find "${package_dir}" -name '._*' -delete

  if [[ "${goos}" == "windows" ]]; then
    cat > "${bin_dir}/slimebot-cli.cmd" <<'EOF'
@echo off
"%~dp0slimebot.exe" cli %*
EOF
    (cd "${DIST_DIR}" && zip -qr "${archive}" "${name}")
  else
    cat > "${bin_dir}/slimebot-cli" <<'EOF'
#!/usr/bin/env sh
exec "$(dirname "$0")/slimebot" cli "$@"
EOF
    chmod +x "${bin_dir}/slimebot" "${bin_dir}/slimebot-cli" "${package_dir}/install.sh" "${package_dir}/uninstall.sh"
    (cd "${DIST_DIR}" && tar -czf "${archive}" "${name}")
  fi
done

echo "Release packages written to ${DIST_DIR}"
