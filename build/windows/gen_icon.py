"""Generate the TauShell icon (appicon.png, appicon.ico) and the Windows
resource object cmd/taushell/rsrc_windows_amd64.syso that embeds it.

Requires Pillow. Run from the repository root: python build\\windows\\gen_icon.py
The icon group is resource ID 3, which is what the Wails window loads.
"""
import io
import struct
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = Path(__file__).resolve().parents[2]
OUT_DIR = Path(__file__).resolve().parent
SYSO = ROOT / "cmd" / "taushell" / "rsrc_windows_amd64.syso"
SIZES = [16, 24, 32, 48, 64, 128, 256]
GROUP_ICON_ID = 3
FONT = "C:/Windows/Fonts/consolab.ttf"


def render(size=1024):
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    mask = Image.new("L", (size, size), 0)
    pad = int(size * 0.04)
    radius = int(size * 0.22)
    ImageDraw.Draw(mask).rounded_rectangle((pad, pad, size - pad, size - pad), radius, fill=255)

    # Diagonal dark gradient background.
    grad = Image.new("RGBA", (size, size))
    px = grad.load()
    top, bottom = (28, 38, 50), (9, 12, 18)
    for y in range(size):
        for x in range(size):
            t = (x + y) / (2 * size)
            px[x, y] = tuple(int(top[i] + (bottom[i] - top[i]) * t) for i in range(3)) + (255,)
    img.paste(grad, (0, 0), mask)

    # Subtle green rim.
    rim = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ImageDraw.Draw(rim).rounded_rectangle(
        (pad, pad, size - pad, size - pad), radius, outline=(111, 211, 155, 150), width=int(size * 0.018))
    img = Image.alpha_composite(img, rim)

    # Tau glyph with a soft glow, plus a terminal cursor block.
    font = ImageFont.truetype(FONT, int(size * 0.80))
    glyph = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glyph)
    box = gd.textbbox((0, 0), "\u03c4", font=font)
    gw, gh = box[2] - box[0], box[3] - box[1]
    gx, gy = (size - gw) / 2 - box[0] - size * 0.04, (size - gh) / 2 - box[1] - size * 0.06
    green = (111, 211, 155, 255)
    gd.text((gx, gy), "\u03c4", font=font, fill=green)
    cw, ch = int(size * 0.11), int(size * 0.035)
    cx = int(gx + box[2] + size * 0.02)
    cy = int(gy + box[3] - ch)
    gd.rectangle((cx, cy, cx + cw, cy + ch), fill=green)

    glow = glyph.filter(ImageFilter.GaussianBlur(size * 0.03))
    img = Image.alpha_composite(img, glow)
    img = Image.alpha_composite(img, glyph)
    return img


def png_bytes(img):
    buf = io.BytesIO()
    img.save(buf, "PNG")
    return buf.getvalue()


def build_ico(images):
    header = struct.pack("<HHH", 0, 1, len(images))
    entries, blobs, offset = b"", b"", 6 + 16 * len(images)
    for size, data in images:
        entries += struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(data), offset)
        blobs += data
        offset += len(data)
    return header + entries + blobs


def build_syso(images):
    """COFF .rsrc object: RT_ICON per image plus one RT_GROUP_ICON (ID 3)."""
    group = struct.pack("<HHH", 0, 1, len(images))
    for i, (size, data) in enumerate(images, 1):
        group += struct.pack("<BBBBHHIH", size % 256, size % 256, 0, 0, 1, 32, len(data), i)

    # (type, id, data) leaves, sorted by type then id.
    leaves = [(3, i, data) for i, (_, data) in enumerate(images, 1)] + [(14, GROUP_ICON_ID, group)]
    types = sorted({t for t, _, _ in leaves})

    def dir_size(n):
        return 16 + 8 * n

    root_size = dir_size(len(types))
    type_sizes = {t: dir_size(sum(1 for lt, _, _ in leaves if lt == t)) for t in types}
    # Each leaf: language dir (1 entry), then a data entry.
    offset = root_size + sum(type_sizes.values())
    leaf_off = {}
    for t, i, _ in leaves:
        leaf_off[(t, i)] = offset
        offset += dir_size(1) + 16
    data_start = (offset + 7) & ~7

    body, relocs = bytearray(), []

    def directory(entries):
        return struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(entries)) + b"".join(
            struct.pack("<II", ident, off) for ident, off in entries)

    body += directory([(t, 0x80000000 | (root_size + sum(type_sizes[x] for x in types if x < t))) for t in types])
    for t in types:
        body += directory([(i, 0x80000000 | leaf_off[(lt, i)]) for lt, i, _ in leaves if lt == t])

    blob_off, blobs = data_start, b""
    for t, i, data in leaves:
        base = leaf_off[(t, i)]
        body += directory([(0x409, base + dir_size(1))])
        relocs.append(len(body))
        body += struct.pack("<IIII", blob_off, len(data), 0, 0)
        blobs += data + b"\0" * (-len(data) % 8)
        blob_off += len(data) + (-len(data) % 8)
    body += b"\0" * (data_start - len(body)) + blobs

    section_off = 20 + 40
    reloc_off = section_off + len(body)
    sym_off = reloc_off + 10 * len(relocs)
    coff = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_off, 1, 0, 0)
    section = struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(body), section_off, reloc_off, 0,
                          len(relocs), 0, 0x40000040)
    reloc_table = b"".join(struct.pack("<IIH", r, 0, 3) for r in relocs)  # ADDR32NB against symbol 0
    symbol = struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)
    return coff + section + bytes(body) + reloc_table + symbol + struct.pack("<I", 4)


def main():
    master = render()
    images = [(s, png_bytes(master.resize((s, s), Image.LANCZOS))) for s in SIZES]
    master.resize((512, 512), Image.LANCZOS).save(OUT_DIR / "appicon.png")
    (OUT_DIR / "appicon.ico").write_bytes(build_ico(images))
    SYSO.write_bytes(build_syso(images))
    print("wrote", OUT_DIR / "appicon.png", OUT_DIR / "appicon.ico", SYSO)


if __name__ == "__main__":
    main()
