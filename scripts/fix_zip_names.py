#!/usr/bin/env python3
"""Rewrite release zips so non-ASCII names carry the UTF-8 flag.
macOS `zip` stores UTF-8 bytes without the flag, which garbles Chinese
filenames in some Windows unzip tools. Python's zipfile sets it correctly."""
import glob
import os
import sys
import zipfile

def fix(path):
    tmp = path + ".tmp"
    with zipfile.ZipFile(path) as zin:
        items = [(info, zin.read(info.filename)) for info in zin.infolist()]
    with zipfile.ZipFile(tmp, "w", zipfile.ZIP_DEFLATED) as zout:
        for info, data in items:
            name = info.filename
            if not (info.flag_bits & 0x800):
                try:
                    name = name.encode("cp437").decode("utf-8")
                except (UnicodeEncodeError, UnicodeDecodeError):
                    pass
            zi = zipfile.ZipInfo(name, date_time=info.date_time)
            zi.compress_type = zipfile.ZIP_DEFLATED
            zi.external_attr = info.external_attr
            zi.internal_attr = info.internal_attr
            zi.create_system = info.create_system
            zout.writestr(zi, data)
    os.replace(tmp, path)
    print("fixed", os.path.basename(path))

for p in sorted(glob.glob("release/*.zip")):
    fix(p)
