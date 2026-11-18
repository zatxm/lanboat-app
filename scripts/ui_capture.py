"""Capture PNGs from an isolated X11 test display."""
import ctypes as C
from pathlib import Path
import struct
import zlib


class XImage(C.Structure):
    _fields_ = [('width', C.c_int), ('height', C.c_int), ('xoffset', C.c_int),
                ('format', C.c_int), ('data', C.c_void_p), ('byte_order', C.c_int),
                ('bitmap_unit', C.c_int), ('bitmap_bit_order', C.c_int),
                ('bitmap_pad', C.c_int), ('depth', C.c_int),
                ('bytes_per_line', C.c_int), ('bits_per_pixel', C.c_int),
                ('red_mask', C.c_ulong), ('green_mask', C.c_ulong), ('blue_mask', C.c_ulong)]


class Display:
    def __init__(self, name):
        self.x = C.CDLL('libX11.so.6')
        self.x.XOpenDisplay.argtypes = [C.c_char_p]
        self.x.XOpenDisplay.restype = C.c_void_p
        self.x.XDefaultRootWindow.argtypes = [C.c_void_p]
        self.x.XDefaultRootWindow.restype = C.c_ulong
        self.x.XGetImage.argtypes = [C.c_void_p, C.c_ulong, C.c_int, C.c_int,
                                   C.c_uint, C.c_uint, C.c_ulong, C.c_int]
        self.x.XGetImage.restype = C.POINTER(XImage)
        self.x.XDestroyImage.argtypes = [C.POINTER(XImage)]
        self.x.XFlush.argtypes = [C.c_void_p]
        self.x.XCloseDisplay.argtypes = [C.c_void_p]
        self.connection = self.x.XOpenDisplay(name.encode())
        if not self.connection:
            raise RuntimeError('cannot open isolated X display')
        self.root = self.x.XDefaultRootWindow(self.connection)

    def read(self, x, y, width, height):
        pointer = self.x.XGetImage(self.connection, self.root, x, y, width, height, 0xffffffff, 2)
        if not pointer:
            raise RuntimeError('cannot read test display')
        try:
            im = pointer.contents
            if im.bits_per_pixel != 32:
                raise RuntimeError('expected 32-bit test display')
            return C.string_at(im.data, im.bytes_per_line * height), im.bytes_per_line
        finally:
            self.x.XDestroyImage(pointer)

    def screenshot(self, path, width=1100, height=850):
        data, stride = self.read(0, 0, width, height)
        scanlines = bytearray()
        for row in range(height):
            scanlines.append(0)
            start = row * stride
            for offset in range(start, start + width * 4, 4):
                scanlines.extend((data[offset + 2], data[offset + 1], data[offset]))
        def chunk(kind, content):
            return struct.pack('>I', len(content)) + kind + content + struct.pack('>I', zlib.crc32(kind + content))
        png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
        png += chunk(b'IDAT', zlib.compress(scanlines)) + chunk(b'IEND', b'')
        Path(path).write_bytes(png)

    def close(self):
        self.x.XCloseDisplay(self.connection)

