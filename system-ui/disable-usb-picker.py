#!/usr/bin/env python3
"""Disable only the USB picker's CoreStartable.start in this exact ROM APK.
Keep APK layout and certificate blocks intact for its system-partition deployment.
This is a modified system APK, not a newly signed installable application.
"""
import hashlib,struct,zlib,zipfile,json
from pathlib import Path
root=Path(__file__).resolve().parents[1];out=root/'work/usb-dialog'
src=out/'SystemUI.apk';raw=bytearray(src.read_bytes())
assert hashlib.sha256(raw).hexdigest()=='6c8ec67aa532dea06915c64ac54a8490e6fc37e6cd5890b661f07c5e7c6f3163', 'ROM APK changed; re-audit required'
with zipfile.ZipFile(src) as z:
 entry=z.getinfo('classes2.dex');assert entry.compress_type==0
 start=entry.header_offset+30+struct.unpack_from('<H',raw,entry.header_offset+26)[0]+struct.unpack_from('<H',raw,entry.header_offset+28)[0]
 dex=bytearray(z.read(entry))
# Audited dexdump: UsbModePickerDialogDelegate.start()V, insns_size=32.
code=0x5c0a9c;insns=code+16
assert struct.unpack_from('<I',dex,code+12)[0]==32
assert dex[insns:insns+4]==bytes.fromhex('549001d0')
dex[insns:insns+64]=b'\x0e\x00'+bytes(62) # return-void plus NOPs; preserve method size and instruction boundaries.
dex[12:32]=hashlib.sha1(dex[32:]).digest()
struct.pack_into('<I',dex,8,zlib.adler32(dex[12:])&0xffffffff)
crc=zlib.crc32(dex)&0xffffffff
raw[start:start+len(dex)]=dex
struct.pack_into('<I',raw,entry.header_offset+14,crc)
eocd=raw.rfind(b'PK\x05\x06');central=struct.unpack_from('<I',raw,eocd+16)[0]
pos=central;found=0
while raw[pos:pos+4]==b'PK\x01\x02':
 n,e,c=struct.unpack_from('<HHH',raw,pos+28)
 if raw[pos+46:pos+46+n]==b'classes2.dex':struct.pack_into('<I',raw,pos+16,crc);found+=1
 pos+=46+n+e+c
assert found==1
result=out/'SystemUI-no-usb-picker.apk';result.write_bytes(raw)
with zipfile.ZipFile(result) as z:assert z.testzip() is None
(out/'patch.json').write_text(json.dumps({'original_sha256':hashlib.sha256(src.read_bytes()).hexdigest(),'patched_sha256':hashlib.sha256(raw).hexdigest(),'method':'com.android.systemui.usb.UsbModePickerDialogDelegate.start()V','change':'return immediately; no USB-state receiver or keyguard callback registered','apk_signature_note':'Original certificate block preserved; modified bytes are not covered by a valid original APK content digest; deploy only as a reversible rooted system overlay, not package installation.'},indent=2)+'\n')
print('Patched only USB picker startup; ZIP integrity verified.')
