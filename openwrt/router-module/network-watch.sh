#!/system/bin/sh
set -eu
MODDIR=${0%/*}
DS=/data/local/Droidspaces/bin/droidspaces
BB=/data/local/Droidspaces/bin/busybox
MODE=${1:-check}
P=$($DS --name=DroidWRT pid 2>/dev/null || true)
case "$P" in ''|NONE|*[!0-9]*) P=;; esac
if [ "$MODE" = check ]; then
 [ -n "$P" ] && [ -d "/proc/$P" ] || { echo missing; exit; }
 STATE=$($BB awk '/^State:/{print $2}' "/proc/$P/status")
 case "$STATE" in T|t|Z|D) echo init; exit;; esac
 # The host timeout remains available even when the container is stuck.
 $BB timeout 12 "$DS" --name=DroidWRT run /usr/sbin/droidwrt-health check || echo init
 exit
fi
[ "$MODE" = repair ] || exit 2
REASON=${2:-missing}
# Physical USB interfaces can return to Android when a netns disappears.
# Android must never route these clients around the container's VPN policy.
for IPT in iptables ip6tables; do
 for D in -i -o; do
  "$IPT" -w -C FORWARD "$D" usb+ -j DROP 2>/dev/null || "$IPT" -w -I FORWARD 1 "$D" usb+ -j DROP
 done
done
if [ -n "$P" ]; then
 for IPT in iptables ip6tables; do
  $BB nsenter -t "$P" -n "$IPT" -w -C FORWARD -i dwr-lan -m comment --comment dwr-recovery -j DROP 2>/dev/null ||
   $BB nsenter -t "$P" -n "$IPT" -w -I FORWARD 1 -i dwr-lan -m comment --comment dwr-recovery -j DROP
 done
fi
case "$REASON" in
 missing|init)
  # Resume a stopped init before asking the runtime for its normal shutdown.
  if [ -n "$P" ]; then kill -CONT "$P" 2>/dev/null || true; fi
  $BB timeout 20 "$DS" --name=DroidWRT stop >/dev/null 2>&1 || true
  # Never start a second instance if the old one did not stop.
  OLD=$($DS --name=DroidWRT pid 2>/dev/null || true)
  case "$OLD" in ''|NONE|*[!0-9]*) ;; *) [ ! -d "/proc/$OLD" ] || exit 1;; esac
  ;;
 netifd|dnsmasq|odhcpd|uhttpd|policy)
  $BB timeout 20 "$DS" --name=DroidWRT run /usr/sbin/droidwrt-health repair "$REASON"
  ;;
 *) exit 2;;
esac
case "$REASON" in
 missing|init|netifd) DROIDWRT_SUPERVISED=1 /system/bin/sh "$MODDIR/start-router.sh";;
esac
# routerMonitor reconstructs uplink, VPN policy and LAN attachments, then removes
# the recovery drop only at the end of a successful policy transaction.
