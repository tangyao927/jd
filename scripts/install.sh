#!/bin/sh
set -eu

start_marker='# >>> jd initialize >>>'
end_marker='# <<< jd initialize <<<'
bind='jd'
shell_name=''
binary_source=''
source_build=0
release_version='latest'
version_set=0
dry_run=0
uninstall=0
purge=0
assume_yes=0

usage() {
  printf '%s\n' 'Usage: install.sh [--version latest|vX.Y.Z | --source | --binary PATH] [--bind NAME] [--shell zsh|bash|fish] [--dry-run] [--uninstall] [--purge --yes]'
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bind) bind=${2-}; shift 2 ;;
    --shell) shell_name=${2-}; shift 2 ;;
    --binary) binary_source=${2-}; shift 2 ;;
    --source) source_build=1; shift ;;
    --version) release_version=${2-}; version_set=1; shift 2 ;;
    --dry-run) dry_run=1; shift ;;
    --uninstall) uninstall=1; shift ;;
    --purge) purge=1; shift ;;
    --yes) assume_yes=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'jd installer: unknown option %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ "$source_build" -eq 1 ] && [ -n "$binary_source" ]; then
  printf '%s\n' 'jd installer: --source and --binary cannot be used together' >&2
  exit 2
fi
if [ "$version_set" -eq 1 ] && { [ "$source_build" -eq 1 ] || [ -n "$binary_source" ]; }; then
  printf '%s\n' 'jd installer: --version cannot be combined with --source or --binary' >&2
  exit 2
fi
if [ "$release_version" != latest ] && ! printf '%s\n' "$release_version" | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/ { valid = 1 } END { exit !valid }'; then
  printf 'jd installer: invalid version %s; use latest or vX.Y.Z\n' "$release_version" >&2
  exit 2
fi

case "$bind" in
  ''|[0-9]*|*[!A-Za-z0-9_]*) printf 'jd installer: invalid binding %s\n' "$bind" >&2; exit 2 ;;
esac

if [ -z "$shell_name" ]; then
  shell_name=$(basename "${SHELL:-zsh}")
fi
case "$shell_name" in
  zsh|bash|fish) ;;
  *) printf 'jd installer: unsupported shell %s\n' "$shell_name" >&2; exit 2 ;;
esac

bin_dir=${JD_INSTALL_BIN_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}
destination=$bin_dir/jd
if [ -n "${JD_INSTALL_PROFILE:-}" ]; then
  profile=$JD_INSTALL_PROFILE
else
  case "$shell_name" in
    zsh) profile=$HOME/.zshrc ;;
    bash)
      if [ "$(uname -s)" = Darwin ] && [ -f "$HOME/.bash_profile" ]; then profile=$HOME/.bash_profile; else profile=$HOME/.bashrc; fi
      ;;
    fish) profile=${XDG_CONFIG_HOME:-$HOME/.config}/fish/config.fish ;;
  esac
fi
data_dir=${JD_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/jd}
config_dir=${JD_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/jd}

remove_managed_block() {
  source_file=$1
  output_file=$2
  if [ ! -f "$source_file" ]; then : > "$output_file"; return; fi
  awk -v start="$start_marker" -v end="$end_marker" '
    $0 == start { skip = 1; next }
    $0 == end { skip = 0; next }
    !skip { print }
  ' "$source_file" > "$output_file"
}

write_block() {
  case "$shell_name" in
    zsh)
      quoted_destination=$(printf '%s' "$destination" | sed "s/'/'\\\\''/g")
      printf '%s\n' "$start_marker"
      printf "_JD_INSTALL_BIN='%s'\n" "$quoted_destination"
      printf 'eval "$("$_JD_INSTALL_BIN" init zsh --bind %s)"\n' "$bind"
      printf 'if whence compdef >/dev/null 2>&1; then eval "$("$_JD_INSTALL_BIN" completion zsh --bind %s)"; fi\n' "$bind"
      printf '%s\n' 'unset _JD_INSTALL_BIN'
      printf '%s\n' "$end_marker"
      ;;
    bash)
      quoted_destination=$(printf '%s' "$destination" | sed "s/'/'\\\\''/g")
      printf '%s\n' "$start_marker"
      printf "_JD_INSTALL_BIN='%s'\n" "$quoted_destination"
      printf 'eval "$("$_JD_INSTALL_BIN" init bash --bind %s)"\n' "$bind"
      printf 'eval "$("$_JD_INSTALL_BIN" completion bash --bind %s)"\n' "$bind"
      printf '%s\n' 'unset _JD_INSTALL_BIN'
      printf '%s\n' "$end_marker"
      ;;
    fish)
      quoted_destination=$(printf '%s' "$destination" | sed "s/'/\\\\'/g")
      printf '%s\n' "$start_marker"
      printf "set -g _JD_INSTALL_BIN '%s'\n" "$quoted_destination"
      printf '$_JD_INSTALL_BIN init fish --bind %s | source\n' "$bind"
      printf '$_JD_INSTALL_BIN completion fish --bind %s | source\n' "$bind"
      printf '%s\n' 'set -e _JD_INSTALL_BIN'
      printf '%s\n' "$end_marker"
      ;;
  esac
}

if [ "$uninstall" -eq 1 ]; then
  if [ "$dry_run" -eq 1 ]; then
    printf 'Would remove managed block from %s and binary %s\n' "$profile" "$destination"
    exit 0
  fi
  profile_dir=$(dirname "$profile")
  mkdir -p "$profile_dir"
  temporary_profile=$(mktemp "$profile_dir/.jd-profile.XXXXXX")
  remove_managed_block "$profile" "$temporary_profile"
  mv "$temporary_profile" "$profile"
  rm -f "$destination"
  if [ "$purge" -eq 1 ]; then
    if [ "$assume_yes" -ne 1 ]; then
      if [ ! -t 0 ]; then printf '%s\n' 'jd installer: --purge requires --yes in non-interactive mode' >&2; exit 2; fi
      printf 'Delete jd data in %s and %s? [y/N] ' "$data_dir" "$config_dir"
      read -r answer
      case "$answer" in y|Y|yes|YES) ;; *) printf '%s\n' 'Data preserved.'; exit 0 ;; esac
    fi
    for target in "$data_dir" "$config_dir"; do
      case "$target" in ''|/|"$HOME") printf 'jd installer: refusing unsafe purge target %s\n' "$target" >&2; exit 2 ;; esac
      rm -rf "$target"
    done
  fi
  printf 'Uninstalled jd; restart %s or reload %s.\n' "$shell_name" "$profile"
  exit 0
fi

existing=$(command -v "$bind" 2>/dev/null || true)
if [ -n "$existing" ] && [ "$existing" != "$destination" ] && ! { [ -f "$profile" ] && grep -Fq "$start_marker" "$profile"; }; then
  printf 'jd installer: binding %s already resolves to %s; choose --bind NAME\n' "$bind" "$existing" >&2
  exit 2
fi

if [ "$dry_run" -eq 1 ]; then
  if [ -n "$binary_source" ]; then source_description=$binary_source
  elif [ "$source_build" -eq 1 ]; then source_description='the current source checkout'
  else source_description="jd release $release_version"
  fi
  printf 'Would install %s to %s and update %s:\n' "$source_description" "$destination" "$profile"
  write_block
  exit 0
fi

if [ -z "$binary_source" ]; then
  temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/jd-install.XXXXXX")
  trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM
  binary_source=$temporary_dir/jd
  if [ "$source_build" -eq 1 ]; then
    script_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
    (cd "$script_dir" && env GOTOOLCHAIN=local go build -trimpath -o "$binary_source" ./cmd/jd)
  else
    case "$(uname -s)" in
      Darwin) release_os=darwin ;;
      Linux) release_os=linux ;;
      *) printf 'jd installer: unsupported operating system %s\n' "$(uname -s)" >&2; exit 2 ;;
    esac
    case "$(uname -m)" in
      x86_64|amd64) release_arch=amd64 ;;
      arm64|aarch64) release_arch=arm64 ;;
      *) printf 'jd installer: unsupported architecture %s\n' "$(uname -m)" >&2; exit 2 ;;
    esac
    release_base=${JD_RELEASE_BASE_URL:-https://github.com/tangyao927/jd/releases}
    if [ "$release_version" = latest ]; then
      download_base=$release_base/latest/download
    else
      download_base=$release_base/download/$release_version
    fi
    asset=jd_${release_os}_${release_arch}.tar.gz
    command -v curl >/dev/null 2>&1 || { printf '%s\n' 'jd installer: curl is required to download releases' >&2; exit 1; }
    curl -fsSL "$download_base/$asset" -o "$temporary_dir/$asset"
    curl -fsSL "$download_base/checksums.txt" -o "$temporary_dir/checksums.txt"
    expected=$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$temporary_dir/checksums.txt")
    [ -n "$expected" ] || { printf 'jd installer: checksum missing for %s\n' "$asset" >&2; exit 1; }
    if command -v shasum >/dev/null 2>&1; then
      actual=$(shasum -a 256 "$temporary_dir/$asset" | awk '{print $1}')
    elif command -v sha256sum >/dev/null 2>&1; then
      actual=$(sha256sum "$temporary_dir/$asset" | awk '{print $1}')
    else
      printf '%s\n' 'jd installer: shasum or sha256sum is required to verify releases' >&2
      exit 1
    fi
    [ "$actual" = "$expected" ] || { printf 'jd installer: checksum mismatch for %s\n' "$asset" >&2; exit 1; }
    tar -xzf "$temporary_dir/$asset" -C "$temporary_dir" jd
  fi
fi
if [ ! -f "$binary_source" ]; then printf 'jd installer: binary not found: %s\n' "$binary_source" >&2; exit 2; fi

mkdir -p "$bin_dir" "$(dirname "$profile")"
temporary_binary=$(mktemp "$bin_dir/.jd.XXXXXX")
cp "$binary_source" "$temporary_binary"
chmod 755 "$temporary_binary"
mv "$temporary_binary" "$destination"

had_marker=0
if [ -f "$profile" ] && grep -Fq "$start_marker" "$profile"; then had_marker=1; fi
if [ -f "$profile" ] && [ "$had_marker" -eq 0 ]; then
  cp "$profile" "$profile.jd-backup.$(date +%Y%m%d%H%M%S)"
fi
temporary_profile=$(mktemp "$(dirname "$profile")/.jd-profile.XXXXXX")
remove_managed_block "$profile" "$temporary_profile"
if [ -s "$temporary_profile" ]; then printf '\n' >> "$temporary_profile"; fi
write_block >> "$temporary_profile"
mv "$temporary_profile" "$profile"

if [ "$had_marker" -eq 0 ] && [ -t 0 ]; then
  candidates=''
  for candidate in "$HOME/Projects" "$HOME/projects" "$HOME/Developer" "$HOME/dev" "$HOME/work" "$HOME/src"; do
    if [ -d "$candidate" ]; then candidates="$candidates\n$candidate"; fi
  done
  if [ -n "$candidates" ]; then
    printf 'Detected project roots:%b\nIndex all of them now? [Y/n] ' "$candidates"
    read -r answer
    case "$answer" in n|N|no|NO) ;; *)
      printf '%b\n' "$candidates" | while IFS= read -r candidate; do
        [ -n "$candidate" ] && "$destination" root add "$candidate"
      done
      ;;
    esac
  fi
fi

printf 'Installed jd at %s. Restart %s or reload %s.\n' "$destination" "$shell_name" "$profile"
