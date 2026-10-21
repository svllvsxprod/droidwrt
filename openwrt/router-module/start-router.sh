#!/system/bin/sh
set -eu
MODDIR=${0%/*}
DS=/data/local/Droidspaces/bin/droidspaces
BB=/data/local/Droidspaces/bin/busybox
CONTAINER_PID=$($DS --name=DroidWRT pid 2>/dev/null || true)
case "$CONTAINER_PID" in
    ''|NONE|*[!0-9]*)
        $DS --name=DroidWRT start
        CONTAINER_PID=$($DS --name=DroidWRT pid 2>/dev/null || true)
        ;;
esac
case "$CONTAINER_PID" in ''|NONE|*[!0-9]*) exit 1;; esac
# Recreate the management peer when the container has a new network namespace.
LAST_PID=$(cat "$MODDIR/container-pid" 2>/dev/null || true)
if [ "$LAST_PID" != "$CONTAINER_PID" ]; then
    ip link delete dwr-host 2>/dev/null || true
fi
if ! ip link show dwr-host >/dev/null 2>&1; then
    ip link add dwr-host type veth peer name dwr-mgmt
    ip link set dwr-mgmt netns "$CONTAINER_PID"
fi
ip address replace 172.29.255.1/30 dev dwr-host
ip link set dwr-host up
# Android's policy routing otherwise bypasses the main table for new links.
ip route replace 172.29.255.0/30 dev dwr-host src 172.29.255.1 table 179
if ! ip rule show | grep -q 'to 172.29.255.0/30 lookup 179'; then
    ip rule add priority 17900 to 172.29.255.0/30 lookup 179
fi
# This link is for management only; never let it bypass the VPN path.
for direction in -i -o; do
    iptables -w -C FORWARD "$direction" dwr-host -j DROP 2>/dev/null ||
        iptables -w -I FORWARD 1 "$direction" dwr-host -j DROP
    ip6tables -w -C FORWARD "$direction" dwr-host -j DROP 2>/dev/null ||
        ip6tables -w -I FORWARD 1 "$direction" dwr-host -j DROP
done
$BB nsenter -t "$CONTAINER_PID" -n ip address replace 172.29.255.2/30 dev dwr-mgmt
$BB nsenter -t "$CONTAINER_PID" -n ip link set dwr-mgmt up
$DS --name=DroidWRT run /sbin/ifup management
echo "$CONTAINER_PID" > "$MODDIR/container-pid"
echo 'Management link ready'
[ "${DROIDWRT_SUPERVISED:-0}" = 1 ] && exit 0
if ! "$BB" pgrep -f '^/data/adb/modules/droidwrt_router/local-proxy$' >/dev/null; then
    nohup "$MODDIR/local-proxy" >> /data/local/Droidspaces/Logs/local-proxy.log 2>&1 </dev/null &
fi
