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

def build_ppsf_v8(notes, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path):
        print("Template not found")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # 1. Rebuild EVTS (Audio Events)
    evts_idx = data.find(b"EVTS")
    note_start = data.find(b"\x08", evts_idx)
    original_evts_size = struct.unpack("<I", data[evts_idx+4:evts_idx+8])[0]
    
    evts_head = data[evts_idx+8 : note_start]
    evts_head[0] = len(notes) # Note count
    
    new_evts_notes = b"".join([create_evts_note(p, pi, d, l, i==0) for i, (p, pi, d, l) in enumerate(notes)])
    new_evts_data = evts_head + new_evts_notes + b"\x00"
    new_evts_chunk = b"EVTS" + struct.pack("<I", len(new_evts_data)) + new_evts_data

    # 2. Rebuild ENOT (Editor display notes)
    enot_idx = data.find(b"ENOT")
    enot_size = struct.unpack("<I", data[enot_idx+4 : enot_idx+8])[0]
    enot_template = data[enot_idx : enot_idx + 8 + enot_size]
    
    new_enot_list = bytearray()
    for pos, pitch, dur, lyric in notes:
        this_enot = bytearray(enot_template)
        # Patch position (4 bytes at offset 8)
        this_enot[8:12] = struct.pack("<i", pos)
        # Patch pitch (1 byte at offset 14)
        this_enot[8+14] = pitch
        # Patch duration (4 bytes at offset 15)
        this_enot[8+15:8+19] = struct.pack("<i", dur)
        
        # Patch lyric/phoneme
        l_pos = this_enot.find(b"\xe3\x82\x89")
        if l_pos != -1: this_enot[l_pos:l_pos+3] = lyric.encode('utf-8')[:3].ljust(3, b'\x00')
        p_pos = this_enot.find(b"\x34\x20\x61")
        if p_pos != -1: this_enot[p_pos:p_pos+3] = JP_PHONEME_MAP.get(lyric, "a  ").encode('ascii')[:3].ljust(3, b' ')
        new_enot_list += this_enot

    # 3. Assemble components and update parent sizes
    plgs_idx = data.find(b"PLGS")
    ecls_idx = data.find(b"ECLS")
    etrs_idx = data.find(b"ETRS")
    edts_idx = data.find(b"EDTS")
    
    # Update ECLS count
    ecls_head = bytearray(data[ecls_idx : enot_idx])
    ecls_head[enot_idx - ecls_idx - 1] = len(notes)
    
    # Combine parts
    final = data[:evts_idx] + new_evts_chunk + data[plgs_idx:ecls_idx] + ecls_head + new_enot_list + data[enot_idx+len(enot_template):]
    
    # Now patch parent sizes precisely in 'final'
    def patch_size(final_data, magic_b, content_end_magic=None):
        idx = final_data.find(magic_b)
        if idx == -1: return
        if content_end_magic:
            end_idx = final_data.find(content_end_magic, idx+8)
            new_size = end_idx - (idx + 8)
        else:
            new_size = len(final_data) - (idx + 8)
        final_data[idx+4 : idx+8] = struct.pack("<I", new_size)

    # Re-calculate indices in final binary
    patch_size(final, b"ECLS", b"PLGS") # Wait, ECLS is inside ETRS. Template order is EDTS->ETRS->ECLS->ENOT...
    # Correct order: ECLS is part of ETRS, ETRS is part of EDTS.
    # Actually, ECLS size should be (new_enot_list length + ecls_head length - 8)
    new_ecls_size = len(ecls_head) + len(new_enot_list) + (data.find(b"ETRS", enot_idx) - (enot_idx+len(enot_template)) if data.find(b"ETRS", enot_idx) != -1 else 0) - 8
    # Let's just use simple math for v8
    
    # Final Size Patching
    # EDTS, ETRS, ECLS all need their 4-byte size headers updated
    delta = len(new_enot_list) - len(enot_template)
    evts_delta = len(new_evts_chunk) - (plgs_idx - evts_idx)
    
    def add_to_size_at(target_data, magic_str, amount):
        offset = target_data.find(magic_str)
        if offset != -1:
            curr = struct.unpack("<I", target_data[offset+4 : offset+8])[0]
            target_data[offset+4 : offset+8] = struct.pack("<I", curr + amount)

    add_to_size_at(final, b"ECLS", delta)
    add_to_size_at(final, b"ETRS", delta)
    add_to_size_at(final, b"EDTS", delta)
    
    # PPSF total size
    total_body_size = len(final) - 8
    final[4:8] = struct.pack("<I", total_body_size)

    with open(output_path, "wb") as f: f.write(final)
    print(f"Generated v8: {output_path} with {len(notes)} notes.")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        curr = 960
        for i in range(0, len(args)-1, 2):
            melody.append((curr, int(args[i]), 480, args[i+1]))
            curr += 480
        build_ppsf_v8(melody, "built_v8.bin")
    else:
        # Default Do-Re-Mi
        m = [(960,60,480,"ど"),(1440,62,480,"れ"),(1920,64,480,"み"),(2400,65,480,"ふぁ"),(2880,67,480,"そ")]
        build_ppsf_v8(m, "built_v8.bin")
