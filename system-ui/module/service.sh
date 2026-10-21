#!/system/bin/sh
MODDIR=${0%/*}
APK=/system/system_ext/priv-app/SystemUI/SystemUI.apk
restore_original() {
 touch "$MODDIR/disable"
 if [ "$(getprop ro.build.fingerprint)" = "$(cat "$MODDIR/rom-fingerprint")" ]; then
  mount -o bind "$MODDIR/stock.apk" "$APK"
 else
  umount -l "$APK"
 fi
 p=$(pidof com.android.systemui)
 [ -z "$p" ] || kill -TERM $p
 echo 'Disabled after compatibility or startup check failed' > "$MODDIR/health"
}
if [ "$(getprop ro.build.fingerprint)" != "$(cat "$MODDIR/rom-fingerprint")" ]; then
 restore_original
 exit
fi
last=
stable=0
for attempt in $(seq 1 60); do
 p=$(pidof com.android.systemui)
 if [ -n "$p" ] && [ "$p" = "$last" ] && [ "$(getprop sys.boot_completed)" = 1 ]; then
  stable=$((stable+1))
  if [ "$stable" -ge 5 ]; then
   echo 'SystemUI stable; USB picker suppression active' > "$MODDIR/health"
   exit
  fi
 else
  stable=0
 fi
 last=$p
 sleep 2
done
restore_original
