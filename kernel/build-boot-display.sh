#!/bin/bash
set -euo pipefail
cd /build/kernel
if ! grep -q droidwrt_clear_splash drivers/misc/mediatek/video/mt6785/videox/mtkfb.c; then
 patch --dry-run -p1 < /project/kernel/patches/clear-boot-frame.patch
 patch -p1 < /project/kernel/patches/clear-boot-frame.patch
fi
# Build the bundled DTC source for the build host instead of executing its
# checked-in x86 binary. The final image still retains the baseline DTB below.
python3 - <<'NATIVE_DTC'
from pathlib import Path
p = Path('/build/kernel/scripts/Makefile.lib')
s = p.read_text()
s = s.replace('$(srctree)/scripts/dtc/dtc_overlay -@', '$(objtree)/scripts/dtc/dtc -@')
p.write_text(s)
NATIVE_DTC
mkdir -p /build/router-out /project/work/kernel-build
if [[ ! -f /build/router-out/.configured ]]; then
 cp /project/work/boot-display/baseline/resolved.config /build/router-out/.config
 touch /build/router-out/.configured
fi
python3 /project/kernel/verify_config.py /build/router-out/.config
export KBUILD_BUILD_VERSION=4
export DROIDWRT_KERNEL_TARGET=Image.gz
bash /project/kernel/build.sh
# This experiment changes only framebuffer C code. Preserve the known working
# device tree byte-for-byte instead of substituting the vendor's x86-only DTC.
python3 /project/kernel/append-baseline-dtb.py
