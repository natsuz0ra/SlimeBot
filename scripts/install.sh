#!/usr/bin/env sh
set -eu

SOURCE_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
INSTALL_DIR="${SLIMEBOT_INSTALL_DIR:-"$HOME/.local/share/slimebot"}"
SHIM_DIR="${SLIMEBOT_BIN_DIR:-"$HOME/.local/bin"}"
REPO="${SLIMEBOT_REPO:-natsuz0ra/SlimeBot}"
VERSION="${SLIMEBOT_VERSION:-latest}"
PATH_BLOCK_START="# >>> SlimeBot PATH >>>"
PATH_BLOCK_END="# <<< SlimeBot PATH <<<"
PATH_CONFIG_FILE=""
PATH_WAS_CONFIGURED=0

download_file() {
  url="$1"
  output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$output"
  else
    echo "curl or wget is required to download SlimeBot." >&2
    exit 1
  fi
}

path_contains_dir() {
  dir="$1"
  case ":$PATH:" in
    *":$dir:"*) return 0 ;;
    *) return 1 ;;
  esac
}

shell_config_file() {
  shell_path="${SHELL:-}"
  if [ -n "$shell_path" ]; then
    shell_name="$(basename "$shell_path")"
  else
    shell_name=""
  fi
  case "$shell_name" in
    zsh) echo "$HOME/.zshrc" ;;
    bash) echo "$HOME/.bashrc" ;;
    *) echo "$HOME/.profile" ;;
  esac
}

remove_slimebot_path_block() {
  file="$1"
  [ -f "$file" ] || return 0
  tmp_file="$(mktemp)"
  awk -v start="$PATH_BLOCK_START" -v end="$PATH_BLOCK_END" '
    $0 == start { skip = 1; next }
    $0 == end && skip { skip = 0; next }
    !skip { print }
  ' "$file" > "$tmp_file"
  mv "$tmp_file" "$file"
}

configure_shell_path() {
  if path_contains_dir "$SHIM_DIR"; then
    echo "Command shim directory is already on PATH: $SHIM_DIR"
    return 0
  fi

  PATH_CONFIG_FILE="$(shell_config_file)"
  mkdir -p "$(dirname "$PATH_CONFIG_FILE")"
  touch "$PATH_CONFIG_FILE"
  remove_slimebot_path_block "$PATH_CONFIG_FILE"
  {
    printf '\n%s\n' "$PATH_BLOCK_START"
    printf 'export PATH="%s:$PATH"\n' "$SHIM_DIR"
    printf '%s\n' "$PATH_BLOCK_END"
  } >> "$PATH_CONFIG_FILE"
  PATH_WAS_CONFIGURED=1
  echo "Added command shim directory to PATH in $PATH_CONFIG_FILE"
}

latest_version() {
  tmp_json="$1"
  download_file "https://api.github.com/repos/$REPO/releases/latest" "$tmp_json"
  sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_json" | head -n 1
}

bootstrap_from_release() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$os" in
    darwin) goos="darwin" ;;
    linux) goos="linux" ;;
    *)
      echo "Unsupported OS for install.sh: $os" >&2
      echo "Use install.ps1 on Windows." >&2
      exit 1
      ;;
  esac
  case "$arch" in
    x86_64|amd64) goarch="amd64" ;;
    arm64|aarch64) goarch="arm64" ;;
    *)
      echo "Unsupported architecture: $arch" >&2
      exit 1
      ;;
  esac

  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

  resolved_version="$VERSION"
  if [ "$resolved_version" = "latest" ]; then
    resolved_version="$(latest_version "$tmp_dir/latest.json")"
  fi
  if [ -z "$resolved_version" ]; then
    echo "Unable to resolve SlimeBot release version." >&2
    exit 1
  fi

  package_name="slimebot-$resolved_version-$goos-$goarch"
  archive="$tmp_dir/$package_name.tar.gz"
  url="https://github.com/$REPO/releases/download/$resolved_version/$package_name.tar.gz"

  echo "Downloading SlimeBot $resolved_version for $goos/$goarch..."
  download_file "$url" "$archive"
  tar -xzf "$archive" -C "$tmp_dir"
  exec sh "$tmp_dir/$package_name/install.sh" "$@"
}

if [ ! -x "$SOURCE_DIR/bin/slimebot" ] || [ ! -f "$SOURCE_DIR/cli/cli.cjs" ]; then
  bootstrap_from_release "$@"
fi

mkdir -p "$INSTALL_DIR" "$SHIM_DIR"

rm -rf "$INSTALL_DIR/bin" "$INSTALL_DIR/cli"
cp -R "$SOURCE_DIR/bin" "$SOURCE_DIR/cli" "$INSTALL_DIR/"
if [ -d "$SOURCE_DIR/docs" ]; then
  cp -R "$SOURCE_DIR/docs" "$INSTALL_DIR/"
fi
cp "$SOURCE_DIR/README.md" "$SOURCE_DIR/README.zh-CN.md" "$SOURCE_DIR/LICENSE" "$INSTALL_DIR/"
if [ -f "$SOURCE_DIR/README.en.md" ]; then
  cp "$SOURCE_DIR/README.en.md" "$INSTALL_DIR/"
fi

cat > "$SHIM_DIR/slimebot" <<EOF
#!/usr/bin/env sh
exec "$INSTALL_DIR/bin/slimebot" "\$@"
EOF

cat > "$SHIM_DIR/slimebot-cli" <<EOF
#!/usr/bin/env sh
exec "$INSTALL_DIR/bin/slimebot" cli "\$@"
EOF

chmod +x "$INSTALL_DIR/bin/slimebot" "$SHIM_DIR/slimebot" "$SHIM_DIR/slimebot-cli"
configure_shell_path

mkdir -p "$HOME/.slimebot"
if [ ! -f "$HOME/.slimebot/config.cfg" ]; then
  "$INSTALL_DIR/bin/slimebot" help >/dev/null
  cat > "$HOME/.slimebot/config.cfg" <<'EOF'
SERVER_PORT=6247
FRONTEND_PORT=7391
DB_PATH=~/.slimebot/storage/data.db
SKILLS_ROOT=~/.slimebot/skills
CHAT_UPLOAD_ROOT=~/.slimebot/storage/chat_uploads
WEB_SEARCH_API_KEY=YOUR_TAVILY_API_KEY
JWT_SECRET=CHANGE_ME_TO_A_RANDOM_SECRET
JWT_EXPIRE=21600
EOF
fi

echo "SlimeBot installed to $INSTALL_DIR"
echo "Command shims installed to $SHIM_DIR"
echo "Run: slimebot"
if [ "$PATH_WAS_CONFIGURED" -eq 1 ]; then
  echo "Open a new terminal to use slimebot directly."
  echo "To use slimebot in this terminal now, run: export PATH=\"$SHIM_DIR:\$PATH\""
elif ! command -v slimebot >/dev/null 2>&1; then
  echo "If your shell cannot find slimebot, run: export PATH=\"$SHIM_DIR:\$PATH\""
fi
echo "Before starting the web service, set JWT_SECRET in $HOME/.slimebot/config.cfg"
