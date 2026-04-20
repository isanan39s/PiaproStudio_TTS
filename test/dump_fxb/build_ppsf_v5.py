import struct
import os
import sys

# Mapping based on manual
JP_PHONEME_MAP = {
    "あ": "a", "い": "i", "う": "M", "え": "e", "お": "o",
    "か": "k a", "き": "k' i", "く": "k M", "け": "k e", "こ": "k o",
    "さ": "s a", "し": "S i", "す": "s M", "せ": "s e", "そ": "s o",
    "た": "t a", "ち": "tS i", "つ": "ts M", "て": "t e", "と": "t o",
    "な": "n a", "に": "J i", "ぬ": "n M", "ね": "n e", "の": "n o",
    "は": "h a", "ひ": "C i", "ふ": r"p\ M", "へ": "h e", "ほ": "h o",
    "ま": "m a", "み": "m' i", "む": "m M", "め": "m e", "も": "m o",
    "や": "j a", "ゆ": "j M", "よ": "j o",
    "ら": "4 a", "り": "4' i", "る": "4 M", "れ": "4 e", "ろ": "4 o",
    "わ": "w a", "を": "o", "ん": "n",
    "ど": "d o", "れ": "4 e", "み": "m' i", "ふぁ": r"p\ a", "そ": "s o", "し": "S i"
}

def patch_single_note(pitch, lyric, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path):
        print(f"Error: Template {template_path} not found.")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    phoneme = JP_PHONEME_MAP.get(lyric, "a")
    lyric_b = lyric.encode('utf-8')
    phoneme_b = phoneme.encode('utf-8')

    # --- PART 1: EVTS Patch ---
    evts_idx = data.find(b"EVTS")
    if evts_idx != -1:
        # Note starts with 08
        note_idx = data.find(b"\x08", evts_idx)
        if note_idx != -1:
            # 0xBB6 in raw_state_007d4 is offset 16 from the 0x08 magic
            # Header is 1 byte magic + 2 bytes size + 16 bytes data
            data[note_idx + 3 + 4] = pitch # Pitch at header+4
            
            # Lyric at header+13
            data[note_idx + 3 + 13 : note_idx + 3 + 16] = lyric_b[:3]
            
            # Phoneme at header+18
            data[note_idx + 3 + 18] = len(phoneme_b)
            data[note_idx + 3 + 19 : note_idx + 3 + 22] = b"\x00\x00\x00"
            data[note_idx + 3 + 19 : note_idx + 3 + 19 + len(phoneme_b)] = phoneme_b

    # --- PART 2: EDTS Patch (Editor display) ---
    enot_idx = data.find(b"ENOT")
    if enot_idx != -1:
        # Pitch in ENOT
        # Usually offset 14 from magic "ENOT"
        data[enot_idx + 14] = pitch
        
        # Lyric in ENOT (find "ら" e3 82 89)
        l_idx = data.find(b"\xe3\x82\x89", enot_idx)
        if l_idx != -1:
            data[l_idx : l_idx + 3] = lyric_b[:3]
            
        # Phoneme in ENOT (find "4 a" 34 20 61)
        p_idx = data.find(b"\x34\x20\x61", enot_idx)
        if p_idx != -1:
            data[p_idx - 1] = len(phoneme_b)
            data[p_idx : p_idx + 3] = b"\x00\x00\x00"
            data[p_idx : p_idx + len(phoneme_b)] = phoneme_b

    with open(output_path, "wb") as f:
        f.write(data)
    print(f"Patched 1 note to {output_path}: Pitch={pitch}, Lyric={lyric}")

if __name__ == "__main__":
    if len(sys.argv) >= 3:
        p = int(sys.argv[1])
        l = sys.argv[2]
        patch_single_note(p, l, "built_v5.bin")
    else:
        # Default C4 "ど"
        patch_single_note(60, "ど", "built_v5.bin")
