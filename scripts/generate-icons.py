"""Rasterize the repository's simple boat mark without external image packages."""
from pathlib import Path
import struct,zlib,math
root=Path(__file__).resolve().parents[1]
def inside(x,y,points):
 hit=False
 for a,b in zip(points,points[1:]+points[:1]):
  if (a[1]>y)!=(b[1]>y) and x<(b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0]:hit=not hit
 return hit
def render(size):
 pixels=bytearray()
 for j in range(size):
  for i in range(size):
   rgba=[0.,0.,0.,0.]
   for dy in [.125,.375,.625,.875]:
    for dx in [.125,.375,.625,.875]:
     x,y=(i+dx)*128/size,(j+dy)*128/size
     ax=max(29-x,0,x-99);ay=max(29-y,0,y-99)
     if ax*ax+ay*ay>29*29:continue
     t=(x+y)/256;color=[76*(1-t)+40*t,141*(1-t)+83*t,255*(1-t)+197*t]
     if inside(x,y,[(65,21),(65,85),(29,85)]) or inside(x,y,[(24,92),(105,92),(88,108),(42,108)]):color=[255]*3
     elif inside(x,y,[(73,37),(73,85),(100,85)]):color=[182,220,255]
     wave=113-4*math.sin((x-21)*math.pi/29)
     if 21<=x<=108 and abs(y-wave)<2.3:color=[156,207,255]
     for k in range(3):rgba[k]+=color[k]/16
     rgba[3]+=255/16
   alpha=round(rgba[3]);pixels.extend([round(v*255/alpha) if alpha else 0 for v in rgba[:3]]+[alpha])
 return bytes(pixels)
def png(size,pixels):
 def chunk(k,d):return struct.pack('>I',len(d))+k+d+struct.pack('>I',zlib.crc32(k+d))
 rows=b''.join(b'\0'+pixels[y*size*4:(y+1)*size*4] for y in range(size))
 return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',size,size,8,6,0,0,0))+chunk(b'IDAT',zlib.compress(rows))+chunk(b'IEND',b'')
for size in [16,32,48,64,128,256]:
 pixels=render(size);p=root/f'packaging/icons/{size}x{size}/lanboat.png';p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(png(size,pixels))
