#!/usr/bin/env python3
"""Package the verified local kernel using the audited AnyKernel3 installer."""
import hashlib
import json
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

root = Path(__file__).resolve().parents[1]
build = root / 'work/kernel-build'
check = json.loads((build / 'config-verification.json').read_text())
assert not check['required_missing'] and not check['unresolved_requests']
assert (build / 'embedded.config').read_bytes() == (build / 'resolved.config').read_bytes()
source = root / 'work/downloads/Suzakukernel-V2-perf-20260807-NonKSU.zip'
dest = root / 'work/prepared/DroidWRT-kernel-4.14.357-20261012.zip'
names = ['LICENSE', 'META-INF/com/google/android/update-binary',
         'META-INF/com/google/android/updater-script', 'tools/ak3-core.sh',
         'tools/busybox']
script = '''# AnyKernel3 installer; original framework by osm0sis.
properties() { '
kernel.string=DroidWRT 4.14.357 for begonia
do.devicecheck=1
do.modules=0
do.systemless=1
do.cleanup=1
do.cleanuponabort=0
device.name1=begonia
supported.versions=16
supported.patchlevels=
supported.vendorpatchlevels=
'; }
BLOCK=/dev/block/platform/bootdevice/by-name/boot;
IS_SLOT_DEVICE=0;
RAMDISK_COMPRESSION=auto;
PATCH_VBMETA_FLAG=auto;
# Keep Magisk detection enabled: legacy SAR kernels need its initramfs patch.
. tools/ak3-core.sh;
dump_boot;
write_boot;
'''
with ZipFile(source) as src, ZipFile(root / 'work/downloads/Magisk-v30.7.apk') as mag, ZipFile(dest, 'w', ZIP_DEFLATED) as out:
    for name in names:
        out.writestr(name, src.read(name))
    for binary in ['magiskboot', 'magiskpolicy']:
        out.writestr('tools/' + binary, mag.read('lib/arm64-v8a/lib' + binary + '.so'))
    out.writestr('anykernel.sh', script)
    out.write(build / 'Image.gz-dtb', 'Image.gz-dtb')
with ZipFile(dest) as z:
    assert z.testzip() is None
    assert z.read('Image.gz-dtb') == (build / 'Image.gz-dtb').read_bytes()
sha = hashlib.sha256(dest.read_bytes()).hexdigest()
dest.with_suffix('.zip.sha256').write_text(sha + '  ' + dest.name + '\n')
print(dest.name, sha)
