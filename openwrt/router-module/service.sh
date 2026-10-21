#!/system/bin/sh
MODDIR=${0%/*}
LOGDIR=/data/local/Droidspaces/Logs
mkdir -p "$LOGDIR"
# Bound previous boot logs; the daemon never logs config contents.
[ -f "$LOGDIR/droidwrt-router.log" ] && mv -f "$LOGDIR/droidwrt-router.log" "$LOGDIR/droidwrt-router.previous.log"
exec >> "$LOGDIR/droidwrt-router.log" 2>&1
while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 2; done
rm -f "$MODDIR/maintenance"
# A supervisor crash is also recoverable. The supervisor holds an exclusive lock.
while :; do
    "$MODDIR/local-proxy" --supervise && exit 0
    sleep 5
done
