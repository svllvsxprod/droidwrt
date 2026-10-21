from pathlib import Path
from PIL import Image,ImageDraw,ImageFont
import math,zipfile,io,struct,zlib,hashlib,json
ROOT=Path(__file__).resolve().parent;work=ROOT.parent/'work/system-finish';work.mkdir(exist_ok=True)
W,H=1080,2340;INK=(179,239,204);FONT='/System/Library/Fonts/Avenir Next.ttc'
font=ImageFont.truetype(FONT,92,index=2)
def bezier(points,count=64):
 a,b,c,d=points;return [((1-t)**3*a[0]+3*(1-t)**2*t*b[0]+3*(1-t)*t*t*c[0]+t**3*d[0],(1-t)**3*a[1]+3*(1-t)**2*t*b[1]+3*(1-t)*t*t*c[1]+t**3*d[1]) for t in [i/count for i in range(count+1)]]
def screen(phase=None):
 im=Image.new('RGB',(W,H),'black');draw=ImageDraw.Draw(im);scale=5.4
 def convert(p):return ((p[0]-54)*scale+W/2,(p[1]-54)*scale+1000)
 pts=[(30,39),(30,63)]+bezier([(30,63),(30,75),(46,75),(46,63)])[1:]+[(46,53)]+bezier([(46,53),(46,43),(62,43),(62,53)])[1:]+[(62,63)]+bezier([(62,63),(62,75),(78,75),(78,63)])[1:]+[(78,39)]
 draw.line([convert(p) for p in pts],fill=INK,width=27)
 for p in pts:
  x,y=convert(p);draw.ellipse((x-13,y-13,x+13,y+13),fill=INK)
 for p in [pts[0],pts[-1]]:
  x,y=convert(p);draw.ellipse((x-13,y-13,x+13,y+13),fill=INK)
 x,y=convert((78,30));draw.ellipse((x-21,y-21,x+21,y+21),fill=INK)
 box=draw.textbbox((0,0),'DroidWRT',font=font);draw.text(((W-(box[2]-box[0]))/2,1160),'DroidWRT',font=font,fill=(240,244,238))
 if phase is not None:
  for i in range(3):
   brightness=.22+.78*(.5+.5*math.cos(2*math.pi*(phase-i/3)))**3
   color=tuple(round(x*brightness) for x in INK);x=W/2+(i-1)*36
   draw.ellipse((x-6,1384,x+6,1396),fill=color)
 return im
base=screen();base.save(work/'droidwrt-splash.png')
animation=ROOT/'module/system/media/bootanimation.zip';animation.parent.mkdir(parents=True,exist_ok=True)
with zipfile.ZipFile(animation,'w',compression=zipfile.ZIP_STORED) as z:
 z.writestr('desc.txt',f'{W} {H} 24\np 0 0 loop\n')
 for n in range(48):
  stream=io.BytesIO();screen(n/48).save(stream,format='PNG',optimize=True);z.writestr(f'loop/{n:03}.png',stream.getvalue())
# MediaTek resource partition: retain all other frames and header fields.
source=ROOT.parent/'work/device-backup/logo-original.img';old=source.read_bytes();payload=old[512:]
# A parseable frame table is not proof that the bootloader accepts an image.
# The device's original logo contains signed certificate records after its payload.
# Never rebuild that partition without a verified device-specific signing procedure.
declared_size=struct.unpack_from('<I',old,4)[0]
if b'cert1' in old[512+declared_size:512+declared_size+8192]:
 raise RuntimeError('Signed logo partition: rebuilding is disabled. Preserve the original image and certificates; customize Android bootanimation only.')
count,total=struct.unpack_from('<II',payload);offsets=list(struct.unpack_from('<'+'I'*count,payload,8))
assert count==60 and offsets[0]==8+4*count and struct.unpack_from('<I',old,4)[0]==total
frames=[payload[offsets[i]:offsets[i+1] if i+1<count else total] for i in range(count)]
raw=base.convert('RGBA').tobytes('raw','BGRA')
variants={0:raw}
for i,label in [(1,'UNLOCKED'),(38,'FASTBOOT')]:
 image=base.copy();drawing=ImageDraw.Draw(image);small=ImageFont.truetype(FONT,30 if i==1 else 40,index=5)
 bbox=drawing.textbbox((0,0),label,font=small);drawing.text(((W-(bbox[2]-bbox[0]))/2,100 if i==1 else 1380),label,font=small,fill=(110,135,122))
 variants[i]=image.convert('RGBA').tobytes('raw','BGRA')
for i,replacement in variants.items():
 assert len(zlib.decompress(frames[i]))==len(replacement)==W*H*4
 frames[i]=zlib.compress(replacement,9)
new_offsets=[];cursor=8+4*count
for frame in frames:new_offsets.append(cursor);cursor+=len(frame)
new_payload=struct.pack('<II',count,cursor)+struct.pack('<'+'I'*count,*new_offsets)+b''.join(frames)
new=bytearray(old);struct.pack_into('<I',new,4,cursor);new[512:512+max(total,cursor)]=new_payload+b'\0'*max(0,total-cursor)
assert len(new)==len(old) and new[:4]==old[:4] and new[8:512]==old[8:512]
for i in range(count):
 decoded=zlib.decompress(new_payload[new_offsets[i]:new_offsets[i+1] if i+1<count else cursor])
 previous=zlib.decompress(payload[offsets[i]:offsets[i+1] if i+1<count else total])
 assert decoded==variants[i] if i in variants else decoded==previous
out=work/'logo-droidwrt.img';out.write_bytes(new)
report={'frames':count,'changed_frames':list(variants),'unchanged_frames':count-len(variants),'partition_bytes':len(new),'original_sha256':hashlib.sha256(old).hexdigest(),'new_sha256':hashlib.sha256(new).hexdigest(),'animation_frames':48,'animation_fps':24,'animation_bytes':animation.stat().st_size}
(work/'branding-build.json').write_text(json.dumps(report,indent=2)+'\n');print(report)
