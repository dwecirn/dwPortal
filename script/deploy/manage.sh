#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/../.." && pwd)
ROOT_PREFIX=${DWPORTAL_ROOT_PREFIX:-}
SKIP_SYSTEMD=${DWPORTAL_SKIP_SYSTEMD:-0}
TEST_CONFIG=${DWPORTAL_TEST_CONFIG:-$REPO_ROOT/config/config.yaml}

OPT_DIR="$ROOT_PREFIX/opt/dwportal"
RELEASES_DIR="$OPT_DIR/releases"
CURRENT_LINK="$OPT_DIR/current"
STATE_DIR="$ROOT_PREFIX/var/lib/dwportal"
CATALOG_LINK="$STATE_DIR/catalog.yaml"
UNIT_PATH="$ROOT_PREFIX/etc/systemd/system/dwportal.service"

usage() {
  cat <<'EOF'
Usage:
  manage.sh install --package ARCHIVE
  manage.sh upgrade --package ARCHIVE
  manage.sh sync-config --caddyfile PATH [--base-domain DOMAIN]
  manage.sh uninstall [--purge]

Environment used for non-system tests:
  DWPORTAL_ROOT_PREFIX=/tmp/root   redirect /opt, /etc, and /var/lib
  DWPORTAL_SKIP_SYSTEMD=1          skip user creation and systemctl
  DWPORTAL_TEST_CONFIG=PATH        override repository test config target
EOF
}

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
log() { printf '==> %s\n' "$*"; }

require_root() {
  if [[ -z "$ROOT_PREFIX" && ${EUID:-$(id -u)} -ne 0 ]]; then
    die "system deployment requires root (run with sudo)"
  fi
}

systemctl_if_enabled() {
  if [[ "$SKIP_SYSTEMD" == 1 ]]; then
    log "systemd skipped: systemctl $*"
    return 0
  fi
  systemctl "$@"
}

restart_service() {
  systemctl_if_enabled restart dwportal.service && \
    systemctl_if_enabled is-active --quiet dwportal.service
}

install_service_unit() {
  local service_user service_group rendered
  service_user=${DWPORTAL_SERVICE_USER:-$(stat -c %U -- "$TEST_CONFIG")}
  service_group=${DWPORTAL_SERVICE_GROUP:-$(stat -c %G -- "$TEST_CONFIG")}
  id "$service_user" >/dev/null 2>&1 || die "service user does not exist: $service_user"
  rendered=$(mktemp)
  sed -e "s/^User=.*/User=$service_user/" \
      -e "s/^Group=.*/Group=$service_group/" \
      "$PAYLOAD/dwportal.service" > "$rendered"
  install -m 0644 -- "$rendered" "$UNIT_PATH"
  rm -f -- "$rendered"
}

parse_package_arg() {
  PACKAGE=
  while (($#)); do
    case "$1" in
      --package) (($# >= 2)) || die "--package requires a path"; PACKAGE=$2; shift 2 ;;
      *) die "unknown argument: $1" ;;
    esac
  done
  [[ -n "$PACKAGE" ]] || die "--package is required"
  PACKAGE=$(realpath -- "$PACKAGE")
  [[ -f "$PACKAGE" ]] || die "package not found: $PACKAGE"
}

inspect_package() {
  local archive_entry
  while IFS= read -r archive_entry; do
    case "$archive_entry" in
      /*|..|../*|*/../*|*/..) die "unsafe archive path: $archive_entry" ;;
    esac
  done < <(tar -tzf "$PACKAGE")

  STAGING=$(mktemp -d)
  trap 'rm -rf -- "${STAGING:-}"' EXIT
  tar -xzf "$PACKAGE" -C "$STAGING"
  PAYLOAD=$(find "$STAGING" -mindepth 1 -maxdepth 1 -type d -print -quit)
  [[ -n "$PAYLOAD" && -f "$PAYLOAD/dwportal" && ! -L "$PAYLOAD/dwportal" && -x "$PAYLOAD/dwportal" ]] || die "invalid dwPortal binary"
  [[ -f "$PAYLOAD/dwportal.service" && ! -L "$PAYLOAD/dwportal.service" ]] || die "invalid systemd unit"
  if [[ -f "$PAYLOAD/VERSION" ]]; then
    VERSION=$(tr -d '[:space:]' < "$PAYLOAD/VERSION")
  else
    VERSION=${PAYLOAD##*/dwportal-}
  fi
  [[ "$VERSION" =~ ^[0-9A-Za-z][0-9A-Za-z._-]*$ ]] || die "invalid package version: $VERSION"
  RELEASE_DIR="$RELEASES_DIR/$VERSION"
}

link_test_catalog() {
  [[ -f "$TEST_CONFIG" ]] || die "test config not found: $TEST_CONFIG"
  mkdir -p -- "$STATE_DIR"
  if [[ -e "$CATALOG_LINK" && ! -L "$CATALOG_LINK" ]]; then
    die "$CATALOG_LINK exists and is not a symlink; refusing to replace it"
  fi
  ln -sfn -- "$TEST_CONFIG" "$CATALOG_LINK"
}

activate_release() {
  local temporary_link="$OPT_DIR/.current.new"
  ln -sfn -- "$RELEASE_DIR" "$temporary_link"
  mv -Tf -- "$temporary_link" "$CURRENT_LINK"
}

install_release() {
  local mode=$1 previous=
  inspect_package
  if [[ "$mode" == install && -e "$CURRENT_LINK" ]]; then
    die "dwPortal is already installed; use upgrade"
  fi
  if [[ "$mode" == upgrade ]]; then
    [[ -L "$CURRENT_LINK" ]] || die "dwPortal is not installed; use install"
    previous=$(readlink -f -- "$CURRENT_LINK")
  fi
  [[ ! -e "$RELEASE_DIR" ]] || die "release already exists: $VERSION"

  mkdir -p -- "$RELEASES_DIR" "$(dirname -- "$UNIT_PATH")"
  install -d -m 0755 -- "$RELEASE_DIR"
  install -m 0755 -- "$PAYLOAD/dwportal" "$RELEASE_DIR/dwportal"
  printf '%s\n' "$VERSION" > "$RELEASE_DIR/VERSION"
  link_test_catalog
  install_service_unit
  activate_release

  systemctl_if_enabled daemon-reload
  if [[ "$mode" == install ]]; then
    if ! systemctl_if_enabled enable --now dwportal.service || \
       ! systemctl_if_enabled is-active --quiet dwportal.service; then
      rm -f -- "$CURRENT_LINK"
      die "service failed to start; release remains at $RELEASE_DIR"
    fi
  elif ! restart_service; then
    log "restart failed; rolling back to $previous"
    RELEASE_DIR=$previous
    activate_release
    restart_service || true
    die "upgrade failed and was rolled back"
  fi
  log "$mode complete: dwPortal $VERSION"
}

sync_config() {
  local caddyfile= base_domain=dw.c temporary uid gid
  while (($#)); do
    case "$1" in
      --caddyfile) (($# >= 2)) || die "--caddyfile requires a path"; caddyfile=$2; shift 2 ;;
      --base-domain) (($# >= 2)) || die "--base-domain requires a value"; base_domain=$2; shift 2 ;;
      *) die "unknown argument: $1" ;;
    esac
  done
  [[ -n "$caddyfile" && -r "$caddyfile" ]] || die "readable --caddyfile is required"
  [[ -f "$TEST_CONFIG" ]] || die "test config not found: $TEST_CONFIG"
  temporary=$(mktemp)
  trap 'rm -f -- "${temporary:-}"' EXIT
  python3 "$REPO_ROOT/script/build/caddy_to_config.py" \
    --input "$caddyfile" --output "$temporary" --base-domain "$base_domain"
  uid=$(stat -c %u -- "$TEST_CONFIG")
  gid=$(stat -c %g -- "$TEST_CONFIG")
  install -m 0644 -o "$uid" -g "$gid" -- "$temporary" "$TEST_CONFIG"
  link_test_catalog
  log "catalog synchronized: $TEST_CONFIG"
}

safe_remove_tree() {
  local target=$1 suffix=$2
  [[ -n "$target" && "$target" != / && "$target" == *"$suffix" ]] || die "refusing unsafe removal: $target"
  rm -rf -- "$target"
}

uninstall_portal() {
  local purge=0
  while (($#)); do
    case "$1" in
      --purge) purge=1; shift ;;
      *) die "unknown argument: $1" ;;
    esac
  done
  systemctl_if_enabled disable --now dwportal.service || true
  [[ ! -e "$UNIT_PATH" && ! -L "$UNIT_PATH" ]] || rm -f -- "$UNIT_PATH"
  systemctl_if_enabled daemon-reload
  [[ ! -e "$OPT_DIR" ]] || safe_remove_tree "$OPT_DIR" /opt/dwportal
  if ((purge)); then
    [[ ! -e "$STATE_DIR" ]] || safe_remove_tree "$STATE_DIR" /var/lib/dwportal
    log "uninstall complete; runtime state purged"
  else
    log "uninstall complete; runtime state preserved at $STATE_DIR"
  fi
}

main() {
  require_root
  local command=${1:-}
  [[ -n "$command" ]] || { usage; exit 2; }
  shift
  case "$command" in
    install) parse_package_arg "$@"; install_release install ;;
    upgrade) parse_package_arg "$@"; install_release upgrade ;;
    sync-config) sync_config "$@" ;;
    uninstall) uninstall_portal "$@" ;;
    -h|--help|help) usage ;;
    *) usage; die "unknown command: $command" ;;
  esac
}

main "$@"
