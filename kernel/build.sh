#!/bin/bash
set -euo pipefail
# Runs inside the dedicated note8-kernel-build container.
export PATH=/usr/lib/llvm-19/bin:$PATH
export ARCH=arm64
export KBUILD_BUILD_USER=note8
export KBUILD_BUILD_HOST=router-builder
export KBUILD_BUILD_TIMESTAMP='Mon Oct 12 08:00:00 UTC 2026'
cd /build/kernel
mkdir -p /build/router-out /project/work/kernel-build
args=(O=/build/router-out LLVM=1 LLVM_IAS=1
      CROSS_COMPILE=aarch64-linux-gnu- CLANG_TRIPLE=aarch64-linux-gnu-
      CROSS_COMPILE_ARM32=arm-linux-gnueabi-)
if [[ ! -f /build/router-out/.configured ]]; then
    make "${args[@]}" begonia_user_defconfig
    scripts/kconfig/merge_config.sh -m -O /build/router-out \
        /build/router-out/.config /project/kernel/router.config
    make "${args[@]}" olddefconfig
    python3 /project/kernel/verify_config.py /build/router-out/.config
    touch /build/router-out/.configured
fi
cp /build/router-out/.config /project/work/kernel-build/resolved.config
clang --version > /project/work/kernel-build/compiler.txt
target=${DROIDWRT_KERNEL_TARGET:-Image.gz-dtb}
case "$target" in Image.gz|Image.gz-dtb) ;; *) echo "Unsupported kernel target" >&2; exit 1;; esac
make "${args[@]}" -j6 "$target"
cp "/build/router-out/arch/arm64/boot/$target" /project/work/kernel-build/
cp /build/router-out/System.map /project/work/kernel-build/
sha256sum "/project/work/kernel-build/$target" > "/project/work/kernel-build/$target.sha256"
