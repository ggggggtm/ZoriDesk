# Only the caller's bundle validator knows signatures; this layer never launches an app.
desktop_stat() {
  case "$(/usr/bin/uname -s)" in
    Darwin) /usr/bin/stat -f "$1" "$2" ;;
    *) /usr/bin/stat -c "$3" "$2" ;;
  esac
}

desktop_identity() {
  if [ ! -e "$1" ] && [ ! -L "$1" ]; then printf '%s\n' absent; return; fi
  [ -d "$1" ] && [ ! -L "$1" ] || return 1
  desktop_stat '%d:%i' "$1" '%d:%i'
}

desktop_configure() {
  case "$destination" in /*/ZoriDesk.app) ;; *) return 1 ;; esac
  destination_parent=${destination%/*}
  case "$destination_parent/" in *.[aA][pP][pP]/*) return 1 ;; esac
  [ -d "$destination_parent" ] && [ ! -L "$destination_parent" ] || return 1
  [ "$(CDPATH= cd -- "$destination_parent" && pwd -P)" = "$destination_parent" ] || return 1
  [ "$(desktop_stat '%u' "$destination_parent" '%u')" = "$(/usr/bin/id -u)" ] || return 1
  desktop_parent_mode=$(desktop_stat '%Lp' "$destination_parent" '%a')
  [ $(((desktop_parent_mode % 10) & 2)) -eq 0 ] &&
    [ $((((desktop_parent_mode / 10) % 10) & 2)) -eq 0 ] || return 1
  previous="$destination.previous"
  transaction="$destination_parent/.ZoriDesk.app.transaction"
  incoming="$transaction/incoming"
  transaction_owned=0
  for desktop_legacy in "$transaction".* "$destination_parent"/.ZoriDesk.app.incoming.*; do
    if [ -e "$desktop_legacy" ] || [ -L "$desktop_legacy" ]; then return 1; fi
  done
}

desktop_write_receipt() {
  (umask 077
    printf 'zoridesk-app-transaction-v1 %s %s %s %s %s %s\n' \
      "$transaction_action" "$transaction_pid" "$original_current" \
      "$original_previous" "$incoming_identity" "$transaction_phase" > "$transaction/receipt.new"
  ) && /bin/mv "$transaction/receipt.new" "$transaction/receipt"
}

desktop_valid_identity() {
  case "$1" in
    absent) return 0 ;;
    ''|*[!0-9:]*) return 1 ;;
  esac
  case "$1" in *:*) ;; *) return 1 ;; esac
  case "${1#*:}" in ''|*:*) return 1 ;; esac
  [ -n "${1%%:*}" ]
}

desktop_read_receipt() {
  [ "$transaction" = "${destination%/*}/.ZoriDesk.app.transaction" ] || return 1
  [ -d "$transaction" ] && [ ! -L "$transaction" ] || return 1
  [ "$(desktop_stat '%u:%Lp' "$transaction" '%u:%a')" = "$(/usr/bin/id -u):700" ] || return 1
  [ -f "$transaction/receipt" ] && [ ! -L "$transaction/receipt" ] || return 1
  [ "$(desktop_stat '%u:%Lp:%l' "$transaction/receipt" '%u:%a:%h')" = "$(/usr/bin/id -u):600:1" ] || return 1
  [ "$(wc -c < "$transaction/receipt")" -le 256 ] || return 1
  [ "$(wc -l < "$transaction/receipt")" -eq 1 ] || return 1
  IFS=' ' read -r receipt_version transaction_action transaction_pid original_current \
    original_previous incoming_identity transaction_phase receipt_extra < "$transaction/receipt" || return 1
  [ "$receipt_version" = zoridesk-app-transaction-v1 ] && [ -z "$receipt_extra" ] || return 1
  case "$transaction_action:$transaction_phase" in
    install:prepared|install:active|install:committed|rollback:active|rollback:committed) ;;
    *) return 1 ;;
  esac
  case "$transaction_pid" in ''|0|*[!0-9]*) return 1 ;; esac
  [ "${#transaction_pid}" -le 10 ] || return 1
  desktop_valid_identity "$original_current" && desktop_valid_identity "$original_previous" &&
    desktop_valid_identity "$incoming_identity" || return 1
  for desktop_entry in "$transaction"/* "$transaction"/.[!.]* "$transaction"/..?*; do
    [ -e "$desktop_entry" ] || [ -L "$desktop_entry" ] || continue
    case "${desktop_entry##*/}" in
      receipt|receipt.new|current|previous|incoming|rejected|recovering) ;;
      *) return 1 ;;
    esac
    [ ! -L "$desktop_entry" ] || return 1
  done
}

desktop_validate_locations() {
  for desktop_slot in "$destination" "$previous" "$transaction/current" \
    "$transaction/previous" "$incoming" "$transaction/rejected"; do
    desktop_slot_id=$(desktop_identity "$desktop_slot") || return 1
    [ "$desktop_slot_id" != absent ] || continue
    if [ "$desktop_slot" = "$incoming" ] && [ "$transaction_phase" = prepared ] &&
      [ "$incoming_identity" = absent ]; then
      # A failed copy is disposable only while both original public slots are unchanged.
      [ -z "$(find "$incoming" -type l -print -quit)" ] || return 1
      continue
    fi
    validate_bundle "$desktop_slot" >/dev/null 2>&1 || return 1
    [ "$desktop_slot_id" = "$original_current" ] || [ "$desktop_slot_id" = "$original_previous" ] ||
      [ "$desktop_slot_id" = "$incoming_identity" ] || return 1
  done
}

desktop_find_identity() {
  [ "$1" != absent ] || return 1
  for desktop_candidate in "$destination" "$previous" "$transaction/current" \
    "$transaction/previous" "$transaction/rejected"; do
    if [ "$(desktop_identity "$desktop_candidate")" = "$1" ]; then
      printf '%s\n' "$desktop_candidate"
      return
    fi
  done
  return 1
}

restore_active_transaction() {
  desktop_read_receipt && desktop_validate_locations || return 1
  # Find both originals before moving either; a missing or replaced backup is not success.
  if [ "$original_current" != absent ]; then
    desktop_current_source=$(desktop_find_identity "$original_current") || return 1
  fi
  if [ "$original_previous" != absent ]; then
    desktop_find_identity "$original_previous" >/dev/null || return 1
  fi
  if [ "$(desktop_identity "$destination")" != "$original_current" ]; then
    if [ -e "$destination" ]; then
      [ ! -e "$transaction/rejected" ] && [ ! -L "$transaction/rejected" ] || return 1
      /bin/mv "$destination" "$transaction/rejected" || return 1
    fi
    if [ "$original_current" != absent ]; then
      /bin/mv "$desktop_current_source" "$destination" || return 1
    fi
  fi
  if [ "$(desktop_identity "$previous")" != "$original_previous" ]; then
    [ ! -e "$previous" ] && [ ! -L "$previous" ] || return 1
    desktop_previous_source=$(desktop_find_identity "$original_previous") || return 1
    /bin/mv "$desktop_previous_source" "$previous" || return 1
  fi
  [ "$(desktop_identity "$destination")" = "$original_current" ] &&
    [ "$(desktop_identity "$previous")" = "$original_previous" ]
}

desktop_verify_committed() {
  desktop_read_receipt && desktop_validate_locations || return 1
  if [ "$transaction_action" = install ]; then
    [ "$(desktop_identity "$destination")" = "$incoming_identity" ] &&
      [ "$(desktop_identity "$previous")" = "$original_current" ]
  else
    [ "$(desktop_identity "$destination")" = "$original_previous" ] &&
      [ "$(desktop_identity "$previous")" = "$original_current" ]
  fi
}

cleanup() {
  desktop_exit_status=$?
  trap - EXIT
  trap '' HUP INT TERM
  if [ "${transaction_owned:-0}" -eq 1 ]; then
    desktop_cleanup_ok=0
    if desktop_read_receipt 2>/dev/null; then
      if [ "$transaction_phase" = committed ]; then
        desktop_verify_committed >/dev/null 2>&1 && desktop_cleanup_ok=1
      else
        restore_active_transaction >/dev/null 2>&1 && desktop_cleanup_ok=1
      fi
    fi
    if [ "$desktop_cleanup_ok" -eq 1 ]; then
      /bin/rm -rf "$transaction" 2>/dev/null || desktop_cleanup_ok=0
    else
      # Never remove a transaction that still owns either original bundle.
      /bin/rmdir "$transaction/recovering" 2>/dev/null || true
    fi
    if [ "$desktop_cleanup_ok" -ne 1 ]; then
      echo "app transaction preserved; recovery required with the same destination" >&2
      desktop_exit_status=3
    fi
  fi
  exit "$desktop_exit_status"
}

desktop_begin() {
  transaction_action=$1
  /bin/mkdir -m 700 "$transaction" 2>/dev/null || {
    echo "app transaction already exists; wait for its owner or use --recover" >&2
    return 1
  }
  original_current=$(desktop_identity "$destination") &&
    original_previous=$(desktop_identity "$previous") || {
      /bin/rmdir "$transaction"
      return 1
    }
  for desktop_existing in "$destination" "$previous"; do
    if [ -e "$desktop_existing" ] && ! validate_bundle "$desktop_existing"; then
      /bin/rmdir "$transaction"
      return 1
    fi
  done
  if [ "$original_current" = absent ] && [ "$original_previous" != absent ]; then
    /bin/rmdir "$transaction"
    return 1
  fi
  transaction_pid=$$
  incoming_identity=absent
  transaction_phase=active
  [ "$transaction_action" != install ] || transaction_phase=prepared
  desktop_write_receipt || return 1
  transaction_owned=1
  trap cleanup EXIT
  trap 'exit 98' HUP INT TERM
}

desktop_install() {
  validate_bundle "$source_app" || return 1
  desktop_begin install || return 1
  /bin/cp -pR "$source_app" "$incoming" || return 1
  validate_bundle "$incoming" || return 1
  incoming_identity=$(desktop_identity "$incoming") || return 1
  transaction_phase=active
  desktop_write_receipt || return 1
  if [ "$original_current" != absent ]; then /bin/mv "$destination" "$transaction/current" || return 1; fi
  if [ "$original_previous" != absent ]; then /bin/mv "$previous" "$transaction/previous" || return 1; fi
  /bin/mv "$incoming" "$destination" || return 1
  case "${LOOM_APP_INSTALL_TEST_FAIL_STEP:-}" in after_swap) return 97 ;; esac
  case "${LOOM_APP_INSTALL_TEST_SIGNAL_STEP:-}" in after_swap) kill -TERM "$$"; return 98 ;; esac
  validate_bundle "$destination" || return 1
  if [ "$original_current" != absent ]; then /bin/mv "$transaction/current" "$previous" || return 1; fi
  transaction_phase=committed
  desktop_write_receipt
}

desktop_rollback() {
  [ -e "$destination" ] && [ -e "$previous" ] || return 1
  desktop_begin rollback || return 1
  /bin/mv "$destination" "$transaction/current" || return 1
  /bin/mv "$previous" "$destination" || return 1
  /bin/mv "$transaction/current" "$previous" || return 1
  validate_bundle "$destination" && validate_bundle "$previous" || return 1
  transaction_phase=committed
  desktop_write_receipt
}

desktop_recover() {
  desktop_read_receipt && desktop_validate_locations || return 1
  if [ "$transaction_phase" = committed ]; then
    desktop_verify_committed || return 1
  else
    for desktop_original in "$original_current" "$original_previous"; do
      if [ "$desktop_original" != absent ]; then desktop_find_identity "$desktop_original" >/dev/null || return 1; fi
    done
  fi
  if kill -0 "$transaction_pid" 2>/dev/null || /bin/ps -p "$transaction_pid" -o pid= >/dev/null 2>&1; then
    echo "app transaction owner is still running" >&2
    return 1
  fi
  # Concurrent recoveries must not both move the originals. A killed recovery fails closed.
  /bin/mkdir -m 700 "$transaction/recovering" 2>/dev/null || return 1
  transaction_owned=1
  trap cleanup EXIT
  trap 'exit 98' HUP INT TERM
  transaction_pid=$$
  desktop_write_receipt
}
