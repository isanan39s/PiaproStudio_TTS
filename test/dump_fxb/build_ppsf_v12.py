import struct
import os
import sys

# Mapping to EXACTLY 3 bytes
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
    "ど": "d o ", "れ": "4 e ", "み": "m' i", "ふぁ": r"p\a", "そ": "s o ", "し": "S i "
}

def patch_fixed_7notes(notes, output_path):
    template_path = "raw_state_009c3-b3ドレミファソラシ.bin"
    if not os.path.exists(template_path):
        print(f"Error: Template {template_path} not found.")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # Find ALL 7 note locations in EVTS and ENOT
    evts_idx = data.find(b"EVTS")
    note_indices_evts = []
    curr = evts_idx
    for _ in range(7):
        curr = data.find(b"\x08", curr + 1)
        note_indices_evts.append(curr)

    note_indices_enot = []
    curr = 0
    for _ in range(7):
        curr = data.find(b"ENOT", curr + 1)
        note_indices_enot.append(curr)

    # Patch 7 notes
    for i in range(7):
        if i < len(notes):
            pos, pitch, dur, lyric = notes[i]
            p_str = JP_PHONEME_MAP.get(lyric, "a  ")
            lyric_b = lyric.encode('utf-8')[:3].ljust(3, b'\x00')
            phoneme_b = p_str.encode('ascii')[:3].ljust(3, b' ')
        else:
            # Hide: Move notes to tick 1,000,000 and zero pitch
            pos, pitch, dur = 1000000, 0, 480
            lyric_b, phoneme_b = b"\x00\x00\x00", b"   "

        # 1. Patch EVTS (Audio)
        oe = note_indices_evts[i] + 3
        data[oe:oe+4] = struct.pack("<i", pos)
        data[oe+4] = pitch
        data[oe+7:oe+11] = struct.pack("<i", dur)
        
        # Search for template lyrics near this note
        for t_lyr in [b"\xe3\x83\x89", b"\xe3\x83\xac", b"\xe3\x83\x9f", b"\xe3\x83\x95", b"\xe3\x82\xbd", b"\xe3\x83\xa9", b"\xe3\x82\xb7"]:
            l_pos = data.find(t_lyr, oe, oe + 100)
            if l_pos != -1:
                data[l_pos:l_pos+3] = lyric_b
                # Patch Phoneme (usually 2 bytes after lyric's null)
                p_pos = l_pos + 5 # Distance to "4 a" etc
                data[p_pos : p_pos+3] = phoneme_b
                break

        # 2. Patch ENOT (Display)
        on = note_indices_enot[i] + 8
        data[on:on+4] = struct.pack("<i", pos)
        # Pitch in ENOT template was sometimes offset. Let's use search/replace logic.
        data[on+6] = pitch
        data[on+7:on+11] = struct.pack("<i", dur)
        
        # Patch strings in ENOT
        for t_lyr in [b"\xe3\x83\x89", b"\xe3\x83\xac", b"\xe3\x83\x9f", b"\xe3\x83\x95", b"\xe3\x82\xbd", b"\xe3\x83\xa9", b"\xe3\x82\xb7"]:
            l_pos_n = data.find(t_lyr, on, on + 150)
            if l_pos_n != -1:
                data[l_pos_n:l_pos_n+3] = lyric_b
                # Phoneme search near lyric
                p_start = l_pos_n + 3
                # Find template phonemes like "4 a"
                for t_pho in [b"64 ", b"34 ", b"6d ", b"70 ", b"73 ", b"53 "]: # Hex for initial phoneme bytes
                    p_pos_n = data.find(t_pho, p_start, p_start + 20)
                    if p_pos_n != -1:
                        data[p_pos_n : p_pos_n + 3] = phoneme_b
                        break
                break

    with open(output_path, "wb") as f: f.write(data)
    print(f"Generated v12: {output_path} (Using 7-note template)")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        curr = 960
        for i in range(0, len(args)-1, 2):
            melody.append((curr, int(args[i]), 480, args[i+1]))
            curr += 480
        patch_fixed_7notes(melody, "built_v12.bin")
    else:
        # Default test
        m = [(960,60,480,"ど"),(1440,62,480,"れ"),(1920,64,480,"み")]
        patch_fixed_7notes(m, "built_v12.bin")
