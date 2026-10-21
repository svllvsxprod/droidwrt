#!/usr/bin/env python3
import hashlib
import json
import struct
import zlib
from pathlib import Path

root = Path(__file__).resolve().parents[1]
baseline = root / 'work/boot-display/baseline'
build = root / 'work/kernel-build'
assert (baseline / 'resolved.config').read_bytes() == (build / 'resolved.config').read_bytes(), 'Baseline configuration changed'
old = (baseline / 'Image.gz-dtb').read_bytes()
decoder = zlib.decompressobj(31)
decoder.decompress(old)
assert decoder.eof
dtb = decoder.unused_data
offset = 0
while offset < len(dtb):
    magic, size = struct.unpack_from('>II', dtb, offset)
    assert magic == 0xd00dfeed and size >= 40 and offset + size <= len(dtb)
    offset += size
assert offset == len(dtb) and offset > 0
kernel = (build / 'Image.gz').read_bytes()
decoder = zlib.decompressobj(31)
decoder.decompress(kernel)
assert decoder.eof and not decoder.unused_data
image = kernel + dtb
(build / 'Image.gz-dtb').write_bytes(image)
sha = hashlib.sha256(image).hexdigest()
(build / 'Image.gz-dtb.sha256').write_text(sha + '  Image.gz-dtb\n')
(build / 'dtb-preservation.json').write_text(json.dumps({
    'baseline_sha256': hashlib.sha256(old).hexdigest(),
    'dtb_sha256': hashlib.sha256(dtb).hexdigest(),
    'dtb_bytes': len(dtb), 'image_sha256': sha,
    'config_identical_to_baseline': True,
}, indent=2) + '\n')
print('Built display experiment with byte-identical baseline device tree:', sha)
