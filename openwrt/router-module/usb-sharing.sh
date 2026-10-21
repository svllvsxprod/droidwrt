#!/system/bin/sh
# Router-owned CDC gadget, with independently selectable ADB.
# Never enable Android tethering/NAT.
set -eu
MODE=${1:-off}
DEBUG=${2:-on}
case "$MODE" in off|ncm|ecm) ;; *) exit 2;; esac
case "$DEBUG" in on|off) ;; *) exit 2;; esac
MODDIR=${0%/*}
umask 077
# One bounded diagnostic per reconciliation; never contains VPN credentials.
exec 2> "$MODDIR/usb-last-operation.log"
G=/config/usb_gadget/g1
BB=/data/local/Droidspaces/bin/busybox
DS=/data/local/Droidspaces/bin/droidspaces
P=$($DS --name=DroidWRT pid 2>/dev/null || true)
case "$P" in ''|*[!0-9]*) P=;; esac
CURRENT=$(readlink "$G/configs/b.1/f1" 2>/dev/null || true)
SECOND=$(readlink "$G/configs/b.1/f2" 2>/dev/null || true)
UDC=musb-hdrc
# Each CDC function keeps its own name even after moving to the router namespace.
# Looking up usb0 by name alone can select ECM on Android while NCM owns usb0
# in the container. Always verify the function's device MAC before moving it.
prepare_interface() {
 IFACE=$(cat "$G/functions/$MODE.usb0/ifname")
 case "$IFACE" in usb[0-9]*) ;; *) return 3;; esac
 MAC=$(cat "$G/functions/$MODE.usb0/dev_addr" | $BB tr -d '\000\n')
 case "$MODE" in ncm) TARGET=usb0;; ecm) TARGET=usb1;; esac
 if $BB nsenter -t "$P" -n ip -o link show dev "$IFACE" 2>/dev/null | $BB grep -Fq "link/ether $MAC "; then
  if [ "$IFACE" != "$TARGET" ]; then
   $BB nsenter -t "$P" -n ip link set "$IFACE" down
   $BB nsenter -t "$P" -n ip link set "$IFACE" name "$TARGET"
  fi
 elif ip -o link show dev "$IFACE" 2>/dev/null | $BB grep -Fq "link/ether $MAC "; then
  ip link set "$IFACE" down
  ip addr flush dev "$IFACE"
  if [ "$IFACE" != "$TARGET" ]; then
   ip link set "$IFACE" name "$TARGET"
  fi
  ip link set "$TARGET" netns "$P"
 else
  echo "CDC interface identity mismatch ($MODE)" >&2
  return 3
 fi
 IFACE=$TARGET
 $BB nsenter -t "$P" -n ip link set "$IFACE" master dwr-lan
 $BB nsenter -t "$P" -n ip link set "$IFACE" up
}
# Settings binder calls must not inherit the private diagnostic file as stderr:
# Android rejects that file descriptor when it crosses into system_server.
# A brief bind operation always restores ADB if it fails.
restore_adb() {
 set +e
 [ -z "$(cat "$G/UDC")" ] || echo '' > "$G/UDC"
 stop adbd
 setprop sys.usb.ffs.ready 0
 rm -f "$G/configs/b.1/f1" "$G/configs/b.1/f2"
 ln -s "$G/functions/ffs.adb" "$G/configs/b.1/f1"
 if [ -f "$MODDIR/usb-adb-class" ]; then
  read CLASS SUBCLASS PROTOCOL < "$MODDIR/usb-adb-class"
  echo "$CLASS" > "$G/bDeviceClass"
  echo "$SUBCLASS" > "$G/bDeviceSubClass"
  echo "$PROTOCOL" > "$G/bDeviceProtocol"
 fi
 setprop sys.usb.config adb
 settings put global adb_enabled 1 >/dev/null 2>&1
 start adbd
 start vendor.usb-hal-1-1
}
rollback() {
 status=$?
 echo "USB transaction failed at ${STAGE:-start}: $status" >&2
 cp "$MODDIR/usb-last-operation.log" "$MODDIR/usb-last-failure.log"
 restore_adb
 exit "$status"
}
if [ "$MODE" = off ]; then
 if [ "$DEBUG" = on ]; then
  if [ "${CURRENT##*/}" != ffs.adb ] || [ "$(getprop init.svc.adbd)" != running ] || [ "$(settings get global adb_enabled 2>/dev/null)" != 1 ]; then restore_adb; fi
 else
  # Charge-only mode. The local app and its loopback API remain available.
  STAGE=charge-only
  trap rollback EXIT
  stop vendor.usb-hal-1-1
  [ "$(settings get global adb_enabled 2>/dev/null)" = 0 ] || settings put global adb_enabled 0 >/dev/null 2>&1
  setprop sys.usb.config droidwrt
  [ -z "$(cat "$G/UDC")" ] || echo '' > "$G/UDC"
  stop adbd
  setprop sys.usb.ffs.ready 0
  rm -f "$G/configs/b.1/f1" "$G/configs/b.1/f2"
  trap - EXIT
 fi
 exit 0
fi
test -n "$P"
# Refuse to attach a physical LAN before the policy with terminal drops exists.
$BB nsenter -t "$P" -n iptables -C DWR_GUARD -j DROP
$BB nsenter -t "$P" -n ip6tables -C DWR_GUARD -j DROP
DEBUG_MATCH=yes
if [ "$DEBUG" = on ]; then
 [ "${SECOND##*/}" = ffs.adb ] && [ "$(getprop init.svc.adbd)" = running ] && [ "$(settings get global adb_enabled 2>/dev/null)" = 1 ] || DEBUG_MATCH=no
else
 # Some ROMs keep adbd alive without a USB transport. Its process state
 # must not cause a working pure CDC gadget to disconnect repeatedly.
 [ -z "$SECOND" ] && [ "$(settings get global adb_enabled 2>/dev/null)" = 0 ] || DEBUG_MATCH=no
fi
if [ "${CURRENT##*/}" != "$MODE.usb0" ] || [ "$(cat "$G/UDC")" != "$UDC" ] || [ "$DEBUG_MATCH" = no ]; then
 case "$(getprop sys.usb.config)" in adb|none|droidwrt|droidwrt,adb) ;; *) exit 4;; esac
 if [ ! -f "$MODDIR/usb-adb-class" ]; then
  umask 077
  echo "$(cat "$G/bDeviceClass") $(cat "$G/bDeviceSubClass") $(cat "$G/bDeviceProtocol")" > "$MODDIR/usb-adb-class"
 fi
 STAGE=detach
 trap rollback EXIT
 stop vendor.usb-hal-1-1
 if [ "$DEBUG" = on ]; then desired_adb=1; else desired_adb=0; fi
 [ "$(settings get global adb_enabled 2>/dev/null)" = "$desired_adb" ] || settings put global adb_enabled "$desired_adb" >/dev/null 2>&1
 # ADB's FunctionFS ready event otherwise triggers Android init's adb-only
 # composition in the middle of this transaction. This transient property
 # is reset by restore_adb and by the normal boot USB initialization.
 if [ "$DEBUG" = on ]; then setprop sys.usb.config droidwrt,adb; else setprop sys.usb.config droidwrt; fi
 # Reopen FunctionFS after the old gadget is detached. Otherwise adbd can
 # close its old endpoints just after bind and silently unbind the new CDC.
 [ -z "$(cat "$G/UDC")" ] || echo '' > "$G/UDC"
 stop adbd
 setprop sys.usb.ffs.ready 0
 rm -f "$G/configs/b.1/f1" "$G/configs/b.1/f2"
 mkdir -p "$G/functions/$MODE.usb0"
 STAGE=identity
 # Keep host/device MAC identities across boots so clients retain one network service.
 for field in host_addr dev_addr; do
  saved="$MODDIR/usb-$MODE-$field"
  if [ -f "$saved" ]; then
   value=$(cat "$saved")
   echo "$value" | $BB grep -Eq '^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$'
   echo "$value" > "$G/functions/$MODE.usb0/$field"
  else
   umask 077
   cat "$G/functions/$MODE.usb0/$field" > "$saved"
  fi
 done
 STAGE=compose
 ln -s "$G/functions/$MODE.usb0" "$G/configs/b.1/f1"
 if [ "$DEBUG" = on ]; then
  ln -s "$G/functions/ffs.adb" "$G/configs/b.1/f2"
  echo 0xef > "$G/bDeviceClass"
  echo 2 > "$G/bDeviceSubClass"
  echo 1 > "$G/bDeviceProtocol"
 else
  echo 2 > "$G/bDeviceClass"
  echo 0 > "$G/bDeviceSubClass"
  echo 0 > "$G/bDeviceProtocol"
 fi
 echo '' > "$G/strings/0x409/manufacturer"
 echo DroidWRT > "$G/strings/0x409/product"
 if [ "$DEBUG" = on ]; then
  STAGE=adb-ready
  start adbd
  tries=0
  until [ "$(getprop sys.usb.ffs.ready)" = 1 ]; do
   tries=$((tries + 1))
   [ "$tries" -lt 30 ] || exit 5
   sleep 0.1
  done
 fi
 STAGE=bind
 echo "$UDC" > "$G/UDC"
 STAGE=network
 prepare_interface
 sleep 0.3
 STAGE=verify-bind
 test "$(cat "$G/UDC")" = "$UDC"
else
 prepare_interface
fi
trap - EXIT
echo "$IFACE"
