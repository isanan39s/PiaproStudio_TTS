import struct
import os
import sys

# Mapping to EXACTLY 3 bytes (using spaces if needed)
JP_PHONEME_MAP = {
    "あ": "a  ", "い": "i  ", "う": "M  ", "え": "e  ", "お": "o  ",
    "か": "k a", "き": "k' i", "く": "k M", "け": "k e", "こ": "k o",
    "さ": "s a", "し": "S i", "す": "s M", "せ": "s e", "そ": "s o",
    "た": "t a", "ち": "tS i", "つ": "ts M", "て": "t e", "と": "t o",
    "な": "n a", "に": "J i", "ぬ": "n M", "ね": "n e", "の": "n o",
    "は": "h a", "ひ": "C i", "ふ": r"p\M", "へ": "h e", "ほ": "h o",
    "ま": "m a", "み": "m' i", "む": "m M", "め": "m e", "も": "m o",
    "や": "j a", "ゆ": "j M", "よ": "j o",
    "ら": "4 a", "り": "4' i", "る": "4 M", "れ": "4 e", "ろ": "4 o",
    "わ": "w a", "を": "o  ", "ん": "n  ",
    "ど": "d o ", "れ": "4 e ", "み": "m' i"
}

def patch_perfect_fixed(pitch, lyric, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path):
        print(f"Error: Template {template_path} not found.")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # 1. Prepare 3-byte lyric and phoneme
    lyric_b = lyric.encode('utf-8')[:3].ljust(3, b'\x00')
    phoneme_str = JP_PHONEME_MAP.get(lyric, "a  ")
    phoneme_b = phoneme_str.encode('ascii')[:3].ljust(3, b'\x20')

    # 2. EVTS Patch (Raw Data)
    # Pitch: 0xBB6, Lyric: 0xBC3, Phoneme: 0xBC9
    data[0xBB6] = pitch
    data[0xBC3:0xBC6] = lyric_b
    data[0xBC8] = 3 # Fixed length 3
    data[0xBC9:0xBC9+3] = phoneme_b

    # 3. EDTS -> ENOT Patch (Editor UI)
    enot_idx = data.find(b"ENOT")
    if enot_idx != -1:
        # Pitch in ENOT
        data[enot_idx + 14] = pitch
        # Lyric in ENOT (find "ら" e3 82 89)
        l_idx = data.find(b"\xe3\x82\x89", enot_idx)
        if l_idx != -1:
            data[l_idx : l_idx + 3] = lyric_b
        # Phoneme in ENOT (find "4 a" 34 20 61)
        p_idx = data.find(b"\x34\x20\x61", enot_idx)
        if p_idx != -1:
            data[p_idx : p_idx + 3] = phoneme_b

    with open(output_path, "wb") as f:
        f.write(data)
    print(f"Generated {output_path} (No size changes): Pitch={pitch}, Lyric={lyric}, Phoneme='{phoneme_str}'")

if __name__ == "__main__":
    p = int(sys.argv[1]) if len(sys.argv) > 1 else 60
    l = sys.argv[2] if len(sys.argv) > 2 else "ど"
    patch_perfect_fixed(p, l, "built_v6.bin")
