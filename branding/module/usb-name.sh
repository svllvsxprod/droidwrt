#!/system/bin/sh
# Optional USB manufacturer is empty so hosts show the product name once.
# Keep USB IDs, serial, functions and driver matching unchanged.
G=/config/usb_gadget/g1
test -d "$G/strings/0x409" || exit 0
if [ -z "$(cat "$G/strings/0x409/manufacturer")" ] &&
   [ "$(cat "$G/strings/0x409/product")" = DroidWRT ]; then
    exit 0
fi
UDC=$(cat "$G/UDC")
restore_binding() {
    if [ -n "$UDC" ] && [ -z "$(cat "$G/UDC")" ]; then
        echo "$UDC" > "$G/UDC"
    fi
}
trap restore_binding EXIT
test -z "$UDC" || echo '' > "$G/UDC" || exit 1
echo '' > "$G/strings/0x409/manufacturer" || exit 1
echo DroidWRT > "$G/strings/0x409/product" || exit 1
