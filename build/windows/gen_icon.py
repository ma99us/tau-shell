"""Embed the fixed TauShell appicon.ico in the Windows resource object
cmd/taushell/rsrc_windows_amd64.syso.

Uses only the Python standard library. Run from the repository root:
    python build\\windows\\gen_icon.py [version]
The icon group is resource ID 3, which is what the Wails window loads. The
optional version (for example 1.0.0) fills the exe's file-version details.
"""
import struct
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT_DIR = Path(__file__).resolve().parent
SYSO = ROOT / "cmd" / "taushell" / "rsrc_windows_amd64.syso"
GROUP_ICON_ID = 3


def read_ico(path):
    data = path.read_bytes()
    reserved, kind, count = struct.unpack_from("<HHH", data)
    if reserved != 0 or kind != 1 or count == 0:
        raise ValueError("invalid application icon")
    images = []
    for index in range(count):
        width, height, _, _, planes, bits, length, offset = struct.unpack_from(
            "<BBBBHHII", data, 6 + 16 * index)
        if width != height or planes != 1 or bits != 32 or offset < 6 + 16 * count or offset + length > len(data):
            raise ValueError("unsupported application icon entry")
        images.append((width or 256, data[offset:offset + length]))
    return images


def _pad4(data):
    return data + b"\0" * (-len(data) % 4)


def _block(key, value=b"", children=b"", text=False, value_len=None):
    """One VS_VERSIONINFO node; offsets are relative to the (4-aligned) start."""
    header = struct.pack("<HHH", 0, len(value) if value_len is None else value_len, 1 if text else 0)
    body = _pad4(header + key.encode("utf-16-le") + b"\0\0") + _pad4(value) + children
    body = _pad4(body)
    return struct.pack("<H", len(body)) + body[2:]


def build_version_info(version):
    """RT_VERSION payload. version is "x.y.z" (or "dev" -> 0.0.0.0)."""
    nums = [int(p) for p in version.split("-")[0].split(".") if p.isdigit()][:4]
    nums += [0] * (4 - len(nums))
    ms, ls = (nums[0] << 16) | nums[1], (nums[2] << 16) | nums[3]
    fixed = struct.pack("<13I", 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x40004, 1, 0, 0, 0)
    shown = ".".join(str(n) for n in nums)
    strings = {
        "CompanyName": "TauShell",
        "FileDescription": "TauShell - persistent AI harness sessions",
        "FileVersion": shown,
        "InternalName": "taushell",
        "LegalCopyright": "MIT License",
        "OriginalFilename": "taushell.exe",
        "ProductName": "TauShell",
        "ProductVersion": version,
    }
    entries = b""
    for key, text in strings.items():
        value = text.encode("utf-16-le") + b"\0\0"
        entries += _block(key, value, text=True, value_len=len(value) // 2)
    string_file = _block("StringFileInfo", children=_block("040904B0", children=entries, text=True), text=True)
    var_file = _block("VarFileInfo", children=_block("Translation", struct.pack("<HH", 0x409, 0x4B0)))
    return _block("VS_VERSION_INFO", fixed, string_file + var_file)


def build_syso(images, version_info):
    """COFF .rsrc object: RT_ICON per image, RT_GROUP_ICON (ID 3), RT_VERSION."""
    group = struct.pack("<HHH", 0, 1, len(images))
    for i, (size, data) in enumerate(images, 1):
        group += struct.pack("<BBBBHHIH", size % 256, size % 256, 0, 0, 1, 32, len(data), i)

    # (type, id, data) leaves, sorted by type then id.
    leaves = ([(3, i, data) for i, (_, data) in enumerate(images, 1)] +
              [(14, GROUP_ICON_ID, group), (16, 1, version_info)])
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
    version = sys.argv[1].lstrip("v") if len(sys.argv) > 1 else "dev"
    images = read_ico(OUT_DIR / "appicon.ico")
    SYSO.write_bytes(build_syso(images, build_version_info(version)))
    print("wrote", SYSO)


if __name__ == "__main__":
    main()
