#!/system/bin/sh
# Physical USB Ethernet is independent of the phone's CDC USB gadget.
# Give a LAN adapter exclusively to OpenWrt; Android owns it again in WAN mode.
set -eu
MODE=${1:-off}
case "$MODE" in on|off) ;; *) exit 2;; esac
MODDIR=${0%/*}
BB=/data/local/Droidspaces/bin/busybox
DS=/data/local/Droidspaces/bin/droidspaces
umask 077
P=$($DS --name=DroidWRT pid 2>/dev/null || true)
case "$P" in ''|*[!0-9]*) exit 3;; esac
in_router() { "$BB" nsenter -t "$P" -n "$@"; }
same_device() { in_router ip -o link show dev "$IFACE" 2>/dev/null | "$BB" grep -Fq "link/ether $MAC "; }
physical_usb() {
 test -e "/sys/class/net/$1/device" || return 1
 case "$(readlink -f "/sys/class/net/$1/device")" in */usb*/*) return 0;; *) return 1;; esac
}
if [ "$MODE" = on ]; then
 # If a container stops, physical Ethernet returns to Android. Keep that
 # transient host path closed until the adapter is reclaimed.
 for IPT in iptables ip6tables; do
  "$IPT" -w -N DWR_ETH 2>/dev/null || true
  "$IPT" -w -C DWR_ETH -i eth+ -j DROP 2>/dev/null || "$IPT" -w -A DWR_ETH -i eth+ -j DROP
  "$IPT" -w -C DWR_ETH -o eth+ -j DROP 2>/dev/null || "$IPT" -w -A DWR_ETH -o eth+ -j DROP
  "$IPT" -w -C FORWARD -j DWR_ETH 2>/dev/null || "$IPT" -w -I FORWARD 1 -j DWR_ETH
 done
fi
if [ "$MODE" = off ]; then
 for record in "$MODDIR"/ethernet-owned-eth*; do
  [ -f "$record" ] || continue
  IFACE=${record##*/ethernet-owned-}
  echo "$IFACE" | "$BB" grep -Eq '^eth[0-9]+$' || { IFACE=; continue; }
  MAC=$(cat "$record")
  if same_device; then
   in_router ip link set "$IFACE" down
   in_router ip link set "$IFACE" nomaster
   in_router ip address flush dev "$IFACE"
   in_router ip link set "$IFACE" netns 1
   ip link set "$IFACE" up
  fi
  rm -f "$record"
 done
 for IPT in iptables ip6tables; do
  if "$IPT" -w -C FORWARD -j DWR_ETH 2>/dev/null; then "$IPT" -w -D FORWARD -j DWR_ETH; fi
 done
fi
IFACE=
if [ "$MODE" = on ]; then
 # A container restart returns physical interfaces to Android. Reclaim by
 # name and MAC on every reconciliation, including after unplug/replug.
 for record in "$MODDIR"/ethernet-owned-eth*; do
  [ -f "$record" ] || continue
  IFACE=${record##*/ethernet-owned-}
  echo "$IFACE" | "$BB" grep -Eq '^eth[0-9]+$' || { IFACE=; continue; }
  MAC=$(cat "$record")
  if same_device; then break; fi
  IFACE=
 done
fi
if [ -z "$IFACE" ]; then
 for candidate in /sys/class/net/eth*; do
  name=${candidate##*/}
  echo "$name" | "$BB" grep -Eq '^eth[0-9]+$' || { IFACE=; continue; }
  physical_usb "$name" || continue
  IFACE=$name
  break
 done
fi
if [ -z "$IFACE" ]; then
 echo '{"state":"absent","interface":"","carrier":false}'
 exit 0
fi
if [ "$MODE" = on ]; then
 # Never connect a client to a container whose terminal VPN guards are absent.
 in_router iptables -C DWR_GUARD -j DROP
 in_router ip6tables -C DWR_GUARD -j DROP
 in_router ip link show dwr-lan >/dev/null
 if ip link show dev "$IFACE" >/dev/null 2>&1; then
  physical_usb "$IFACE" || exit 4
  MAC=$(cat "/sys/class/net/$IFACE/address")
  # Save ownership before moving, so an interrupted operation can be retried.
  echo "$MAC" > "$MODDIR/ethernet-owned-$IFACE.next"
  mv "$MODDIR/ethernet-owned-$IFACE.next" "$MODDIR/ethernet-owned-$IFACE"
  ip link set "$IFACE" down
  ip address flush dev "$IFACE"
  ip link set "$IFACE" netns "$P"
 fi
 in_router ip link set "$IFACE" master dwr-lan
 in_router ip link set "$IFACE" up
 LINK=$(in_router ip -o link show dev "$IFACE")
 STATE=lan
else
 LINK=$(ip -o link show dev "$IFACE")
 STATE=wan
fi
CARRIER=false
echo "$LINK" | "$BB" grep -q LOWER_UP && CARRIER=true
printf '{"state":"%s","interface":"%s","carrier":%s}\n' "$STATE" "$IFACE" "$CARRIER"
