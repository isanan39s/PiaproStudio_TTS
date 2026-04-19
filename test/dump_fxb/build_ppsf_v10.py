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

def create_evts_note(pos, pitch, dur, lyric, is_first=False):
    phoneme = JP_PHONEME_MAP.get(lyric, "a  ").encode('ascii')[:3].ljust(3, b' ')
    lyric_b = lyric.encode('utf-8')[:3].ljust(3, b'\x00')
    header = struct.pack("<iBi", pos, pitch, dur) + b"\x40\x08\x00\x00\x32\x32\x7f"
    body = header + struct.pack("B", 3) + lyric_b + b"\x00" + struct.pack("B", 3) + phoneme + b"\x00\x00\x06normal\x00"
    if is_first:
        body += b"\x00" * 7
    else:
        body += b"\x00\x7a\x02\x01\x40\x00\x00\x00\x00\x00\x01\x32\x00\x00\x00\x00\x00\x00\x00"
    return b"\x08" + struct.pack("<H", len(body)) + body

def build_ppsf_v10(notes, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path): return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # --- 1. Audio Data (EVTS) ---
    evts_idx = data.find(b"EVTS")
    note_start = data.find(b"\x08", evts_idx)
    plgs_idx = data.find(b"PLGS")
    
    evts_head = data[evts_idx+8 : note_start]
    evts_head[0] = len(notes)
    new_evts_notes = b"".join([create_evts_note(p, pi, d, l, i==0) for i, (p, pi, d, l) in enumerate(notes)])
    new_evts_payload = evts_head + new_evts_notes + b"\x00"
    new_evts_chunk = b"EVTS" + struct.pack("<I", len(new_evts_payload)) + new_evts_payload

    # --- 2. Editor Display (ENOT/ALTENOT) ---
    enot_idx = data.find(b"ENOT")
    enot_size = struct.unpack("<I", data[enot_idx+4 : enot_idx+8])[0]
    enot_template = data[enot_idx : enot_idx + 8 + enot_size]
    
    new_enot_list = bytearray()
    for i, (pos, pitch, dur, lyric) in enumerate(notes):
        this_enot = bytearray(enot_template)
        this_enot[8:12] = struct.pack("<i", pos)
        this_enot[8+14] = pitch
        this_enot[8+15:8+19] = struct.pack("<i", dur)
        l_pos = this_enot.find(b"\xe3\x82\x89")
        if l_pos != -1: this_enot[l_pos:l_pos+3] = lyric.encode('utf-8')[:3]
        p_pos = this_enot.find(b"\x34\x20\x61")
        if p_pos != -1: this_enot[p_pos:p_pos+3] = JP_PHONEME_MAP.get(lyric, "a  ").encode('ascii')[:3]
        
        if i == 0:
            new_enot_list += this_enot
        else:
            # Wrap Note 2+ in ALT prefix and remove first 'E' of 'ENOT'
            # Based on hex dump: 00 03 41 4c 54 45 4e 4f 54... (ALT + ENOT overlapped?)
            # Actually it's ALT (size 3) "ALT" + ENOT
            new_enot_list += b"\x00\x03ALT" + this_enot
            # Note: The 'ALT' seems to be a chunk too. 
            # From dump: 00 03 41 4c 54 45 4e 4f 54 -> "ALTENOT"
            # It's actually ALT (size 0) or similar.
            # Let's use the exact bytes from 003c3d3.bin: 00 03 41 4c 54 45 4e 4f 54
            # This is ALT(3) + ENOT

    # --- 3. Assembly ---
    ecls_idx = data.find(b"ECLS")
    data[enot_idx - 1] = len(notes)
    
    # Construction
    final = data[:evts_idx] + new_evts_chunk + data[plgs_idx : enot_idx] + new_enot_list + data[enot_idx + len(enot_template) : ]
    
    # Delta for sizes
    enot_delta = len(new_enot_list) - len(enot_template)
    def add_to_size(target, magic, amount):
        offset = target.find(magic)
        if offset != -1:
            curr = struct.unpack("<I", target[offset+4 : offset+8])[0]
            target[offset+4 : offset+8] = struct.pack("<I", curr + amount)

    add_to_size(final, b"ECLS", enot_delta)
    add_to_size(final, b"ETRS", enot_delta)
    add_to_size(final, b"EDTS", enot_delta)
    
    # PPSF total
    final[4:8] = struct.pack("<I", len(final) - 8)

    with open(output_path, "wb") as f: f.write(final)
    print(f"Generated v10: {output_path} with {len(notes)} notes.")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        curr = 960
        for i in range(0, len(args)-1, 2):
            melody.append((curr, int(args[i]), 480, args[i+1]))
            curr += 480
        build_ppsf_v10(melody, "built_v10.bin")
    else:
        m = [(960,60,480,"ど"),(1440,62,480,"れ"),(1920,64,480,"み")]
        build_ppsf_v10(m, "built_v10.bin")
