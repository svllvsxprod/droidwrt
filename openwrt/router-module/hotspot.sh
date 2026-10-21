#!/system/bin/sh
set -eu
umask 077
MODE=${1:-off}
case "$MODE" in on|off) ;; *) exit 2;; esac
MODDIR=${0%/*}
BB=/data/local/Droidspaces/bin/busybox
DS=/data/local/Droidspaces/bin/droidspaces
export CLASSPATH="$MODDIR/router-wifi.dex"
# Only switched Ethernet frames can cross the host; routed AP traffic cannot bypass OpenWrt.
for IPT in iptables ip6tables; do
 "$IPT" -w -N DWR_AP 2>/dev/null || true
 "$IPT" -w -C DWR_AP -i dwr-ap -o dwr-ap -m physdev --physdev-is-bridged -j ACCEPT 2>/dev/null || "$IPT" -w -I DWR_AP 1 -i dwr-ap -o dwr-ap -m physdev --physdev-is-bridged -j ACCEPT
 for IFACE in ap+ dwr-ap; do
  for DIR in -i -o; do
   "$IPT" -w -C DWR_AP "$DIR" "$IFACE" -j DROP 2>/dev/null || "$IPT" -w -A DWR_AP "$DIR" "$IFACE" -j DROP
  done
 done
 "$IPT" -w -C FORWARD -j DWR_AP 2>/dev/null || "$IPT" -w -I FORWARD 1 -j DWR_AP
done
stop_owned() {
 app_process /system/bin RouterWifi stop >/dev/null
 ip link delete dwr-ap-h 2>/dev/null || true
 ip link delete dwr-ap 2>/dev/null || true
 rm -f "$MODDIR/hotspot-owned" "$MODDIR/hotspot-pid" "$MODDIR/hotspot-applied.json"
}
if [ "$MODE" = off ]; then
 [ ! -f "$MODDIR/hotspot-owned" ] || stop_owned
 exit 0
fi
P=$($DS --name=DroidWRT pid)
case "$P" in ''|*[!0-9]*) exit 3;; esac
$BB nsenter -t "$P" -n iptables -C DWR_GUARD -j DROP
$BB nsenter -t "$P" -n ip6tables -C DWR_GUARD -j DROP
# Restart only when the user applies changed radio settings.
if [ -f "$MODDIR/hotspot-owned" ]; then
 if [ ! -f "$MODDIR/hotspot-applied.json" ]; then
  stop_owned
 else
  # The bundled BusyBox has no cmp applet. Android's Toybox provides it.
  # Avoid -s: Android cmp collapses I/O errors to exit 1 in silent mode.
  # Only a real content difference may restart the radio; comparison errors must not.
  COMPARE=0
  /system/bin/cmp "$MODDIR/hotspot-config.json" "$MODDIR/hotspot-applied.json" >/dev/null 2>&1 || COMPARE=$?
  case "$COMPARE" in
   0) ;;
   1) stop_owned ;;
   *) echo 'Unable to compare Wi-Fi configuration; keeping the current radio' >&2; exit 5 ;;
  esac
 fi
fi
# Clear Android's own tethering request before starting only the Wi-Fi radio.
if [ ! -f "$MODDIR/hotspot-owned" ] || ! ip link show ap0 >/dev/null 2>&1; then
 cmd wifi stop-softap >/dev/null 2>&1 || true
 app_process /system/bin RouterWifi stop >/dev/null
 touch "$MODDIR/hotspot-owned"
 trap stop_owned EXIT
 app_process /system/bin RouterWifi start >/dev/null
 COUNT=15
 while ! ip link show ap0 2>/dev/null | $BB grep -q LOWER_UP; do
  COUNT=$((COUNT-1)); [ "$COUNT" -gt 0 ] || exit 4; sleep 1
 done
fi
# If Android takes IP ownership later, release its DHCP/NAT service explicitly.
if ip -4 addr show dev ap0 | $BB grep -q 'inet '; then
 app_process /system/bin RouterWifi untether ap0 >/dev/null
fi
ip -4 addr flush dev ap0
ip link show dwr-ap >/dev/null 2>&1 || ip link add dwr-ap type bridge
LAST=$(cat "$MODDIR/hotspot-pid" 2>/dev/null || true)
if [ "$LAST" != "$P" ] || ! $BB nsenter -t "$P" -n ip link show dwr-ap-l >/dev/null 2>&1; then
 ip link delete dwr-ap-h 2>/dev/null || true
 ip link add dwr-ap-h type veth peer name dwr-ap-l
 ip link set dwr-ap-l netns "$P"
fi
$BB nsenter -t "$P" -n ip link set dwr-ap-l master dwr-lan
$BB nsenter -t "$P" -n ip link set dwr-ap-l up
ip link set dwr-ap-h master dwr-ap
ip link set ap0 master dwr-ap
sysctl -w net.ipv6.conf.dwr-ap.disable_ipv6=1 net.ipv6.conf.ap0.disable_ipv6=1 >/dev/null
ip link set dwr-ap-h up
ip link set dwr-ap up
echo "$P" > "$MODDIR/hotspot-pid"
cp "$MODDIR/hotspot-config.json" "$MODDIR/hotspot-applied.json.next"
mv "$MODDIR/hotspot-applied.json.next" "$MODDIR/hotspot-applied.json"
trap - EXIT
echo ready
