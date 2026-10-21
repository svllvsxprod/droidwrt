#!/system/bin/sh
MODDIR=${0%/*}
while [ "$(getprop sys.boot_completed)" != 1 ]; do sleep 2; done
while IFS= read -r package; do
    case "$package" in ''|'#'*) continue;; esac
    pm disable-user --user 0 "$package" >/dev/null 2>&1
    am force-stop --user 0 "$package" >/dev/null 2>&1
done < "$MODDIR/packages.txt"
settings put global power_button_long_press 1
settings put global power_button_very_long_press 1

# SMS is retained for the future DroidWRT message inbox.
pm enable --user 0 com.android.messaging >/dev/null 2>&1
cmd role add-role-holder --user 0 android.app.role.SMS com.android.messaging >/dev/null 2>&1
# A dedicated router needs no scanning for nearby Bluetooth/NFC devices.
settings put global ble_scan_always_enabled 0
settings put global wifi_scan_always_enabled 0

# A static appliance UI keeps system transitions immediate.
settings put global window_animation_scale 0
settings put global transition_animation_scale 0
settings put global animator_duration_scale 0
# Preserve the Quickstep service: Android gestures depend on it.
cmd package set-home-activity --user 0 dev.droidwrt.panel/.MainActivity >/dev/null 2>&1
locksettings set-disabled true >/dev/null 2>&1
am start --user 0 -a android.intent.action.MAIN -c android.intent.category.HOME -n dev.droidwrt.panel/.MainActivity >/dev/null 2>&1

# Disable advertising APIs and their scheduled work without removing APEX frameworks.
cmd device_config put adservices global_kill_switch true
cmd device_config put adservices adservices_enabled false
