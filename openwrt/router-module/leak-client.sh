#!/system/bin/sh
# Attach an isolated diagnostic client to the same LAN bridge as real clients.
set -eu
P=$1
C=$2
case "$P:$C" in *[!0-9:]*|:*) exit 2;; esac
BB=/data/local/Droidspaces/bin/busybox
$BB nsenter -t "$P" -n ip link delete dwr-check-l 2>/dev/null || true
ip link delete dwr-check-c 2>/dev/null || true
ip link add dwr-check-c type veth peer name dwr-check-l
ip link set dwr-check-l netns "$P"
ip link set dwr-check-c netns "$C"
$BB nsenter -t "$P" -n ip link set dwr-check-l master dwr-lan
$BB nsenter -t "$P" -n ip link set dwr-check-l up
$BB nsenter -t "$C" -n ip link set lo up
$BB nsenter -t "$C" -n ip link set dwr-check-c up
$BB nsenter -t "$C" -n ip addr add 192.168.8.3/24 dev dwr-check-c
$BB nsenter -t "$C" -n ip route add default via 192.168.8.1 dev dwr-check-c
$BB nsenter -t "$C" -n sysctl -w net.ipv6.conf.all.disable_ipv6=0 >/dev/null
$BB nsenter -t "$C" -n ip -6 addr add fd57:6477:7274:1::3/64 dev dwr-check-c nodad
$BB nsenter -t "$C" -n ip -6 route add default via fd57:6477:7274:1::1 dev dwr-check-c
