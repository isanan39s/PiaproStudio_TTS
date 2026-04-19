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

def build_ppsf_v9(notes, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path):
        print("Template not found")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # --- 1. Rebuild EVTS (Audio Data) ---
    evts_idx = data.find(b"EVTS")
    note_08 = data.find(b"\x08", evts_idx)
    plgs_idx = data.find(b"PLGS")
    
    evts_prefix = data[evts_idx+8 : note_08]
    evts_prefix[0] = len(notes) # Update note count byte
    
    new_evts_notes = b"".join([create_evts_note(p, pi, d, l, i==0) for i, (p, pi, d, l) in enumerate(notes)])
    new_evts_payload = evts_prefix + new_evts_notes + b"\x00"
    new_evts_chunk = b"EVTS" + struct.pack("<I", len(new_evts_payload)) + new_evts_payload
    
    evts_delta = len(new_evts_chunk) - (plgs_idx - evts_idx)

    # --- 2. Rebuild ENOTs (Editor Data) ---
    enot_idx = data.find(b"ENOT")
    enot_size = struct.unpack("<I", data[enot_idx+4 : enot_idx+8])[0]
    enot_template = data[enot_idx : enot_idx + 8 + enot_size]
    
    new_enot_list = bytearray()
    for pos, pitch, dur, lyric in notes:
        this_enot = bytearray(enot_template)
        this_enot[8:12] = struct.pack("<i", pos)
        this_enot[8+14] = pitch
        this_enot[8+15:8+19] = struct.pack("<i", dur)
        l_pos = this_enot.find(b"\xe3\x82\x89")
        if l_pos != -1: this_enot[l_pos:l_pos+3] = lyric.encode('utf-8')[:3]
        p_pos = this_enot.find(b"\x34\x20\x61")
        if p_pos != -1: this_enot[p_pos:p_pos+3] = JP_PHONEME_MAP.get(lyric, "a  ").encode('ascii')[:3]
        new_enot_list += this_enot

    enot_delta = len(new_enot_list) - len(enot_template)

    # --- 3. Assemble and Update Sizes ---
    ecls_idx = data.find(b"ECLS")
    # Note count in ECLS is just before first ENOT
    data[enot_idx - 1] = len(notes)
    
    # Construction: [Start:EVTS] + [New EVTS] + [PLGS:ENOT] + [New ENOTs] + [ENOT_End:EOF]
    final = data[:evts_idx] + new_evts_chunk + data[plgs_idx : enot_idx] + new_enot_list + data[enot_idx + len(enot_template) : ]
    
    # Patch parent sizes in 'final'
    def add_to_size(target, magic, amount):
        offset = target.find(magic)
        if offset != -1:
            curr = struct.unpack("<I", target[offset+4 : offset+8])[0]
            target[offset+4 : offset+8] = struct.pack("<I", curr + amount)

    add_to_size(final, b"ECLS", enot_delta)
    add_to_size(final, b"ETRS", enot_delta)
    add_to_size(final, b"EDTS", enot_delta)
    
    # PPSF total body size
    body_size = len(final) - 8
    final[4:8] = struct.pack("<I", body_size)

    with open(output_path, "wb") as f: f.write(final)
    print(f"Built v9: {output_path} with {len(notes)} notes. EVTS delta={evts_delta}, ENOT delta={enot_delta}")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        curr = 960
        for i in range(0, len(args)-1, 2):
            melody.append((curr, int(args[i]), 480, args[i+1]))
            curr += 480
        build_ppsf_v9(melody, "built_v9.bin")
    else:
        m = [(960,60,480,"ど"),(1440,62,480,"れ"),(1920,64,480,"み")]
        build_ppsf_v9(m, "built_v9.bin")
