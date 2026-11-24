#!/bin/sh
set -eu

usage() {
  echo "usage: install-loom-local-app.sh [--dry-run] --app PATH --destination /absolute/path/ZoriDesk.app" >&2
  echo "       install-loom-local-app.sh --rollback --destination /absolute/path/ZoriDesk.app" >&2
  echo "       install-loom-local-app.sh --recover --destination /absolute/path/ZoriDesk.app" >&2
  exit 2
}

dry_run=0
rollback=0
recover=0
source_app=
destination=

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      dry_run=1
      shift
      ;;
    --rollback)
      rollback=1
      shift
      ;;
    --recover)
      recover=1
      shift
      ;;
    --app)
      [ "$#" -ge 2 ] || usage
      source_app=$2
      shift 2
      ;;
    --destination)
      [ "$#" -ge 2 ] || usage
      destination=$2
      shift 2
      ;;
    *)
      usage
      ;;
  esac
done
[ "$recover" -eq 0 ] || { [ "$rollback" -eq 0 ] && [ "$dry_run" -eq 0 ] && [ -z "$source_app" ]; } || usage
[ -n "$destination" ] || usage
case "$destination" in
  /*/ZoriDesk.app) ;;
  *) usage ;;
esac
[ "$destination" = "${destination%/}" ] || usage

effective_uid=$(/usr/bin/id -u)
destination_parent=$(/usr/bin/dirname "$destination")
case "$destination_parent/" in
  *.[aA][pP][pP]/*) echo "destination must not be inside an application bundle" >&2; exit 3 ;;
esac
[ -d "$destination_parent" ] && [ ! -L "$destination_parent" ] || {
  echo "invalid destination parent" >&2
  exit 3
}
[ "$(CDPATH= cd -- "$destination_parent" && pwd -P)" = "$destination_parent" ] || {
  echo "non-canonical destination parent" >&2
  exit 3
}
[ "$(/usr/bin/stat -f '%u' "$destination_parent")" = "$effective_uid" ] || {
  echo "destination parent owner mismatch" >&2
  exit 3
}
parent_mode=$(/usr/bin/stat -f '%Lp' "$destination_parent")
[ $(((parent_mode % 10) & 2)) -eq 0 ] || {
  echo "world-writable destination parent" >&2
  exit 3
}

previous="$destination.previous"
for controlled in "$destination" "$previous"
do
  [ ! -L "$controlled" ] || {
    echo "symlinked app target" >&2
    exit 3
  }
  if [ -e "$controlled" ]; then
    [ -d "$controlled" ] || {
      echo "invalid app target" >&2
      exit 3
    }
    [ "$(/usr/bin/stat -f '%u' "$controlled")" = "$effective_uid" ] || {
      echo "app target owner mismatch" >&2
      exit 3
    }
  fi
done
if [ "$recover" -eq 0 ] && [ -d "$previous" ] && [ ! -d "$destination" ]; then
  echo "rollback app exists without current app" >&2
  exit 3
fi

validate_bundle() {
  bundle=$1
  [ -d "$bundle" ] && [ ! -L "$bundle" ] || return 1
  [ -z "$(find "$bundle" -type l -print -quit)" ] || return 1
  [ "$(/usr/bin/stat -f '%u' "$bundle")" = "$effective_uid" ] || return 1
  executable="$bundle/Contents/MacOS/LoomLocalApp"
  helper="$bundle/Contents/Library/Helpers/loomd"
  agent_plist="$bundle/Contents/Library/LaunchAgents/io.github.ggggggtm.zoridesk.local.daemon.plist"
  plist="$bundle/Contents/Info.plist"
  [ -f "$executable" ] && [ ! -L "$executable" ] || return 1
  [ -f "$helper" ] && [ ! -L "$helper" ] || return 1
  [ -f "$agent_plist" ] && [ ! -L "$agent_plist" ] || return 1
  [ -f "$plist" ] && [ ! -L "$plist" ] || return 1
  [ "$(/usr/bin/stat -f '%Lp' "$bundle")" = 700 ] || return 1
  [ "$(/usr/bin/stat -f '%Lp' "$executable")" = 700 ] || return 1
  [ "$(/usr/bin/stat -f '%Lp' "$helper")" = 700 ] || return 1
  [ "$(/usr/bin/stat -f '%Lp' "$agent_plist")" = 600 ] || return 1
  [ "$(/usr/bin/stat -f '%Lp' "$plist")" = 600 ] || return 1
  [ -z "$(
    find "$bundle" -exec /usr/bin/stat -f '%u' {} \; |
      /usr/bin/grep -Ev "^${effective_uid}$" || true
  )" ] || return 1
  [ -z "$(
    find "$bundle" -type d -exec /usr/bin/stat -f '%Lp' {} \; |
      /usr/bin/grep -Ev '^700$' || true
  )" ] || return 1
  [ -z "$(
    find "$bundle" -type f ! -path "$executable" ! -path "$helper" \
      -exec /usr/bin/stat -f '%Lp' {} \; |
      /usr/bin/grep -Ev '^600$' || true
  )" ] || return 1
  [ "$(
    /usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist"
  )" = io.github.ggggggtm.zoridesk.local ] || return 1
  /usr/bin/lipo -archs "$executable" |
    /usr/bin/grep -Eq '(^| )arm64( |$)' || return 1
  /usr/bin/lipo -archs "$helper" |
    /usr/bin/grep -Eq '(^| )arm64( |$)' || return 1
  [ "$(
    /usr/libexec/PlistBuddy -c 'Print :BundleProgram' "$agent_plist"
  )" = Contents/Library/Helpers/loomd ] || return 1
  [ "$(/usr/libexec/PlistBuddy -c 'Print :Label' "$agent_plist")" = io.github.ggggggtm.zoridesk.local.daemon ] || return 1
  [ "$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:0' "$agent_plist")" = loomd ] || return 1
  [ "$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:1' "$agent_plist")" = --zoridesk-app-service ] || return 1
  ! /usr/libexec/PlistBuddy -c 'Print :ProgramArguments:2' "$agent_plist" >/dev/null 2>&1 || return 1
  /usr/bin/codesign --verify --deep --strict "$bundle" || return 1
}

# Never replace or delete an unrecognized current/rollback bundle.
for controlled in "$destination" "$previous"; do
  if [ -e "$controlled" ] && ! validate_bundle "$controlled"; then
    echo "foreign or invalid app target" >&2
    exit 3
  fi
done

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)/lib/desktop-install-transaction.sh"
desktop_configure || { echo "invalid app transaction target" >&2; exit 3; }

if [ "$recover" -eq 1 ]; then
  desktop_recover 2>/dev/null || { echo "app recovery unavailable; transaction retained" >&2; exit 3; }
  exit 0
fi

if [ "$rollback" -eq 1 ]; then
  [ "$dry_run" -eq 0 ] && [ -z "$source_app" ] || usage
  validate_bundle "$destination" && validate_bundle "$previous" || {
    echo "rollback unavailable" >&2
    exit 3
  }
  desktop_rollback 2>/dev/null || { echo "app rollback failed" >&2; exit 3; }
  exit 0
fi

[ -n "$source_app" ] || usage
case "$source_app" in
  /*/ZoriDesk.app) ;;
  *) usage ;;
esac
validate_bundle "$source_app" || {
  echo "invalid source app" >&2
  exit 3
}

if [ "$dry_run" -eq 1 ]; then
  exit 0
fi

desktop_install 2>/dev/null || { echo "app installation failed" >&2; exit 3; }
