#!/system/bin/sh
MODDIR=${0%/*}
while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 2; done
settings put global device_name DroidWRT
settings put secure bluetooth_name DroidWRT
settings put global wifi_p2p_device_name DroidWRT
/system/bin/sh "$MODDIR/usb-name.sh"
