#!/usr/bin/env bash
# capture-hardware.sh — dump raw hardware information for Ember's probe fixtures.
#
# Usage: scripts/capture-hardware.sh <machine-slug> [--out DIR] [--notes "text"]
#
# Writes one file per command into testdata/hardware/raw/<machine-slug>/ plus
# meta.json. Commands that are not available produce a <name>.missing file so
# the parsers can be tested against "tool absent" too. No root required; the
# script never modifies the system. Hostname, user name and home directory are
# replaced by placeholders before anything is written.
#
# Supported: Linux (glibc or musl) and macOS. Windows: use capture-hardware.ps1.
# See docs-private/design/probe.md.

set -u

SCRIPT_VERSION="1"

usage() {
  sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
  cat <<'EOF'

What is collected
  Linux:  uname, /proc/cpuinfo, lscpu (json + text), /proc/meminfo, nvidia-smi,
          rocminfo, rocm-smi, /sys/class/drm amdgpu memory files, lspci (VGA/3D),
          lsblk (json), df, /sys/class/power_supply, ip -j link, vulkaninfo summary,
          llama-server --list-devices (if an engine is found)
  macOS:  uname, sw_vers, sysctl -a, system_profiler (Hardware, Displays, Storage,
          Power) as JSON, vm_stat, diskutil list, diskutil info -plist for the
          root volume, pmset -g batt, networksetup -listallhardwareports,
          llama-server --list-devices (if an engine is found)
EOF
}

if [ $# -lt 1 ] || [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
  usage
  [ $# -lt 1 ] && exit 2
  exit 0
fi

SLUG="$1"; shift
OUT=""
NOTES=""
while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    --notes) NOTES="$2"; shift 2 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

case "$SLUG" in
  *[!a-zA-Z0-9._-]*|"") echo "machine slug must match [a-zA-Z0-9._-]+" >&2; exit 2 ;;
esac

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ -z "$OUT" ]; then
  OUT="$REPO_ROOT/testdata/hardware/raw/$SLUG"
fi
mkdir -p "$OUT"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
HOST="$(hostname 2>/dev/null || echo unknown-host)"
USER_NAME="${USER:-$(id -un 2>/dev/null || echo unknown-user)}"
HOME_DIR="${HOME:-/nonexistent}"

# scrub replaces the home directory, host name and user name with placeholders.
# Host and user are matched as whole words so that short names (e.g. "vm") do
# not corrupt tokens like "vmx". perl handles \b portably; the sed fallback uses
# GNU (\b) or BSD ([[:<:]]) word boundaries.
scrub() {
  if command -v perl >/dev/null 2>&1; then
    SCRUB_HOME="$HOME_DIR" SCRUB_HOST="$HOST" SCRUB_USER="$USER_NAME" perl -pe '
      s/\Q$ENV{SCRUB_HOME}\E/<HOME>/g;
      s/\b\Q$ENV{SCRUB_HOST}\E\b/<HOSTNAME>/g;
      s/\b\Q$ENV{SCRUB_USER}\E\b/<USER>/g;'
  elif [ "$OS" = "darwin" ]; then
    sed -e "s#${HOME_DIR}#<HOME>#g" -e "s#[[:<:]]${HOST}[[:>:]]#<HOSTNAME>#g" -e "s#[[:<:]]${USER_NAME}[[:>:]]#<USER>#g"
  else
    sed -e "s#${HOME_DIR}#<HOME>#g" -e "s#\b${HOST}\b#<HOSTNAME>#g" -e "s#\b${USER_NAME}\b#<USER>#g"
  fi
}

# run NAME CMD... — runs CMD, writes stdout (scrubbed) to $OUT/NAME, or NAME.missing.
run() {
  name="$1"; shift
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1: command not found" > "$OUT/$name.missing"
    printf '  %-32s missing (%s)\n' "$name" "$1"
    return 0
  fi
  if "$@" > "$OUT/$name.tmp" 2> "$OUT/$name.stderr"; then
    scrub < "$OUT/$name.tmp" > "$OUT/$name"
    rm -f "$OUT/$name.tmp"
    [ -s "$OUT/$name.stderr" ] || rm -f "$OUT/$name.stderr"
    printf '  %-32s ok\n' "$name"
  else
    code=$?
    scrub < "$OUT/$name.tmp" > "$OUT/$name"
    rm -f "$OUT/$name.tmp"
    echo "exit $code" >> "$OUT/$name.stderr"
    printf '  %-32s exit %s (kept output)\n' "$name" "$code"
  fi
}

# copyfile NAME PATH — copies a file (scrubbed) or writes NAME.missing.
copyfile() {
  name="$1"; path="$2"
  if [ -r "$path" ]; then
    scrub < "$path" > "$OUT/$name"
    printf '  %-32s ok\n' "$name"
  else
    echo "$path: not readable" > "$OUT/$name.missing"
    printf '  %-32s missing (%s)\n' "$name" "$path"
  fi
}

find_llama_server() {
  if command -v llama-server >/dev/null 2>&1; then
    command -v llama-server; return 0
  fi
  for base in "${EMBER_HOME:-$HOME_DIR/.ember}/engines" "$HOME_DIR/.ember/engines"; do
    [ -d "$base" ] || continue
    found="$(find "$base" -type f -name 'llama-server' 2>/dev/null | head -n 1)"
    if [ -n "$found" ]; then echo "$found"; return 0; fi
  done
  return 1
}

echo "Capturing hardware info for '$SLUG' ($OS) into $OUT"

run uname.txt uname -a
run date.txt date -u +%Y-%m-%dT%H:%M:%SZ

case "$OS" in
  linux)
    copyfile proc_cpuinfo.txt /proc/cpuinfo
    copyfile proc_meminfo.txt /proc/meminfo
    copyfile os_release.txt /etc/os-release
    run lscpu.json lscpu -J
    run lscpu.txt lscpu
    run nproc.txt nproc
    run nvidia_smi.csv nvidia-smi --query-gpu=index,name,memory.total,memory.used,memory.free,driver_version,compute_cap,pci.bus_id --format=csv
    run nvidia_smi.txt nvidia-smi
    run rocminfo.txt rocminfo
    run rocm_smi.txt rocm-smi --showmeminfo vram --showproductname --json
    run lspci_vga.txt sh -c 'lspci -nn | grep -Ei "vga|3d|display"'
    run lsblk.json lsblk -J -o NAME,TYPE,SIZE,ROTA,TRAN,MOUNTPOINT,MODEL
    run df.txt df -kP
    run ip_link.json ip -j link
    run vulkaninfo_summary.txt vulkaninfo --summary
    # amdgpu / drm memory files and power supply, as a directory dump
    {
      for f in /sys/class/drm/card*/device/mem_info_vram_total /sys/class/drm/card*/device/mem_info_vram_used \
               /sys/class/drm/card*/device/vendor /sys/class/drm/card*/device/device /sys/class/drm/card*/device/uevent; do
        [ -r "$f" ] && { echo "== $f"; cat "$f"; }
      done
    } > "$OUT/sys_class_drm.txt" 2>/dev/null
    {
      for d in /sys/class/power_supply/*; do
        [ -d "$d" ] || continue
        echo "== $d"
        for f in type status capacity online present; do
          [ -r "$d/$f" ] && printf '%s=%s\n' "$f" "$(cat "$d/$f")"
        done
      done
    } > "$OUT/sys_class_power_supply.txt" 2>/dev/null
    printf '  %-32s ok\n' sys_class_drm.txt
    printf '  %-32s ok\n' sys_class_power_supply.txt
    ;;
  darwin)
    run sw_vers.txt sw_vers
    run sysctl_a.txt sysctl -a
    run sysctl_hw.txt sysctl hw machdep.cpu
    run system_profiler_hardware.json system_profiler -json SPHardwareDataType
    run system_profiler_displays.json system_profiler -json SPDisplaysDataType
    run system_profiler_storage.json system_profiler -json SPStorageDataType
    run system_profiler_power.json system_profiler -json SPPowerDataType
    run vm_stat.txt vm_stat
    run diskutil_list.txt diskutil list
    run diskutil_info_root.plist diskutil info -plist /
    run df.txt df -k
    run pmset_batt.txt pmset -g batt
    run networksetup_ports.txt networksetup -listallhardwareports
    ;;
  *)
    echo "unsupported OS: $OS (use capture-hardware.ps1 on Windows)" >&2
    ;;
esac

if LS="$(find_llama_server)"; then
  run llama_list_devices.txt "$LS" --list-devices
  run llama_server_version.txt "$LS" --version
else
  echo "llama-server: not found in PATH or engines dir" > "$OUT/llama_list_devices.txt.missing"
  printf '  %-32s missing (no engine installed yet)\n' llama_list_devices.txt
fi

# meta.json — no personal data; notes are free text supplied by the operator.
esc() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
cat > "$OUT/meta.json" <<EOF
{
  "schema": 1,
  "script": "capture-hardware.sh",
  "script_version": "$SCRIPT_VERSION",
  "machine": "$(esc "$SLUG")",
  "os": "$OS",
  "arch": "$(uname -m)",
  "captured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "notes": "$(esc "$NOTES")"
}
EOF
printf '  %-32s ok\n' meta.json

echo
echo "Done. Review the files in $OUT for anything personal before committing."
echo "Please add RAM, GPU and any oddities to meta.json \"notes\"."
