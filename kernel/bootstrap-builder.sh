#!/bin/bash
set -euo pipefail
# Dedicated disposable toolchain, persistent project-only build volume.
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends clang-19 lld-19 llvm-19 make bc bison flex libssl-dev libelf-dev gcc-aarch64-linux-gnu gcc-arm-linux-gnueabi python3 git cpio kmod patch ca-certificates xz-utils
mkdir -p /build/kernel
if [ ! -f /build/kernel/Makefile ]; then
 tar -xzf /project/work/downloads/infinity-kernel-877a3efe4a336f13d6bb2814869a1e515f7d7c14.tar.gz --strip-components=1 -C /build/kernel
 for name in native-cpio real-ikconfig ip6gre-refcount-begonia; do
  patch -d /build/kernel -p1 < "/project/kernel/patches/$name.patch"
 done
fi
