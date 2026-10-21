#!/system/bin/sh
set -eu
MODDIR=${0%/*}
DS=/data/local/Droidspaces/bin/droidspaces
BB=/data/local/Droidspaces/bin/busybox
CONTAINER_PID=$($DS --name=DroidWRT pid 2>/dev/null || true)
case "$CONTAINER_PID" in ''|NONE|*[!0-9]*) exit 1;; esac
ROUTE=$(ip -4 route get 1.1.1.1)
UPSTREAM=$(echo "$ROUTE" | $BB awk '{for(i=1;i<=NF;i++)if($i=="dev"){print $(i+1);exit}}')
TABLE=$(echo "$ROUTE" | $BB awk '{for(i=1;i<=NF;i++)if($i=="table"){print $(i+1);exit}}')
SOURCE=$(echo "$ROUTE" | $BB awk '{for(i=1;i<=NF;i++)if($i=="src"){print $(i+1);exit}}')
GATEWAY=$(echo "$ROUTE" | $BB awk '{for(i=1;i<=NF;i++)if($i=="via"){print $(i+1);exit}}')
case "$UPSTREAM" in ccmni[0-9]*|wlan[0-9]*|eth[0-9]*|usb[0-9]*) ;; *) exit 1;; esac
case "$TABLE" in "$UPSTREAM") ;; ''|*[!0-9]*) exit 1;; esac
# Guard chains always end in DROP. Updating their children cannot open forwarding.
iptables -w -N DWR_UPLINK 2>/dev/null || true
iptables -w -N DWR_UP_OUT 2>/dev/null || true
iptables -w -N DWR_UP_IN 2>/dev/null || true
iptables -w -C DWR_UPLINK -i dwr-up -j DWR_UP_OUT 2>/dev/null || iptables -w -A DWR_UPLINK -i dwr-up -j DWR_UP_OUT
iptables -w -C DWR_UPLINK -i dwr-up -j DROP 2>/dev/null || iptables -w -A DWR_UPLINK -i dwr-up -j DROP
iptables -w -C DWR_UPLINK -o dwr-up -j DWR_UP_IN 2>/dev/null || iptables -w -A DWR_UPLINK -o dwr-up -j DWR_UP_IN
iptables -w -C DWR_UPLINK -o dwr-up -j DROP 2>/dev/null || iptables -w -A DWR_UPLINK -o dwr-up -j DROP
iptables -w -C FORWARD -j DWR_UPLINK 2>/dev/null || iptables -w -I FORWARD 1 -j DWR_UPLINK
for direction in -i -o; do
 ip6tables -w -C FORWARD "$direction" dwr-up -j DROP 2>/dev/null || ip6tables -w -I FORWARD 1 "$direction" dwr-up -j DROP
done
LAST_PID=$(cat "$MODDIR/uplink-pid" 2>/dev/null || true)
if [ "$LAST_PID" != "$CONTAINER_PID" ]; then ip link delete dwr-up 2>/dev/null || true; fi
if ! ip link show dwr-up >/dev/null 2>&1; then
 ip link add dwr-up type veth peer name dwr-wan
 ip link set dwr-wan netns "$CONTAINER_PID"
fi
ip addr replace 172.29.254.1/30 dev dwr-up
ip link set dwr-up up
ip route replace 172.29.254.0/30 dev dwr-up table 179
ip rule show | $BB grep -q 'to 172.29.254.0/30 lookup 179' || ip rule add pref 17901 to 172.29.254.0/30 lookup 179
ip rule show | $BB grep -q '17921:.*from 172.29.254.2.*unreachable' || ip rule add pref 17921 from 172.29.254.2/32 unreachable
if ! ip rule show | $BB grep -q "17920:.*from 172.29.254.2.*lookup $TABLE "; then
 while ip rule del pref 17920 2>/dev/null; do :; done
 ip rule add pref 17920 from 172.29.254.2/32 lookup "$TABLE"
fi
# Keep established traffic flowing during periodic reconciliation. Never empty
# these child chains: their parent deliberately drops on fall-through.
iptables -w -C DWR_UP_OUT -s 172.29.254.2/32 -o "$UPSTREAM" -j ACCEPT 2>/dev/null || {
 iptables -w -R DWR_UP_OUT 1 -s 172.29.254.2/32 -o "$UPSTREAM" -j ACCEPT 2>/dev/null || iptables -w -A DWR_UP_OUT -s 172.29.254.2/32 -o "$UPSTREAM" -j ACCEPT
}
iptables -w -C DWR_UP_IN -i "$UPSTREAM" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || {
 iptables -w -R DWR_UP_IN 1 -i "$UPSTREAM" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || iptables -w -A DWR_UP_IN -i "$UPSTREAM" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
}
iptables -w -t nat -N DWR_UP_NAT 2>/dev/null || true
iptables -w -t nat -C POSTROUTING -j DWR_UP_NAT 2>/dev/null || iptables -w -t nat -I POSTROUTING 1 -j DWR_UP_NAT
iptables -w -t nat -C DWR_UP_NAT -s 172.29.254.2/32 -o "$UPSTREAM" -j MASQUERADE 2>/dev/null || {
 iptables -w -t nat -R DWR_UP_NAT 1 -s 172.29.254.2/32 -o "$UPSTREAM" -j MASQUERADE 2>/dev/null || iptables -w -t nat -A DWR_UP_NAT -s 172.29.254.2/32 -o "$UPSTREAM" -j MASQUERADE
}
sysctl -w net.ipv4.ip_forward=1 >/dev/null
$BB nsenter -t "$CONTAINER_PID" -n ip address replace 172.29.254.2/30 dev dwr-wan
$BB nsenter -t "$CONTAINER_PID" -n ip link set dwr-wan up
$BB nsenter -t "$CONTAINER_PID" -n ip route replace default via 172.29.254.1 dev dwr-wan
echo "$CONTAINER_PID" > "$MODDIR/uplink-pid"
# Roaming/DHCP changes can keep the interface and routing table unchanged.
# Transport recovery and leak checks must still see the new network identity.
echo "$UPSTREAM $TABLE src=$SOURCE via=$GATEWAY" > "$MODDIR/uplink-state"
echo 'Container uplink ready'
