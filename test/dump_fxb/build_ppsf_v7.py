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

def update_chunk_size(data, magic_name):
    idx = data.find(magic_name)
    if idx == -1: return
    # Find next chunk or end of parent to determine size
    # This is a bit risky, but for EDTS/ETRS/ECLS/EVTS it works if they are in order
    # Simplest: size is determined by the content we just built
    pass

def build_ppsf_v7(notes, output_path):
    template_path = "raw_state_007d4.bin"
    if not os.path.exists(template_path):
        print("Template not found")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    # 1. EVTS Rebuild
    evts_idx = data.find(b"EVTS")
    note_08 = data.find(b"\x08", evts_idx)
    evts_prefix = data[evts_idx+8 : note_08]
    evts_prefix[0] = len(notes) # Update note count
    
    new_evts_body = b"".join([create_evts_note(p, pi, d, l, i==0) for i, (p, pi, d, l) in enumerate(notes)])
    evts_chunk_data = evts_prefix + new_evts_body + b"\x00"
    
    # 2. ENOT Rebuild
    enot_idx = data.find(b"ENOT")
    enot_size = struct.unpack("<I", data[enot_idx+4 : enot_idx+8])[0]
    enot_template = data[enot_idx : enot_idx + 8 + enot_size]
    
    new_enot_list = bytearray()
    for pos, pitch, dur, lyric in notes:
        this_enot = bytearray(enot_template)
        this_enot[8+14] = pitch
        l_pos = this_enot.find(b"\xe3\x82\x89")
        if l_pos != -1: this_enot[l_pos:l_pos+3] = lyric.encode('utf-8')[:3]
        p_pos = this_enot.find(b"\x34\x20\x61")
        if p_pos != -1: this_enot[p_pos:p_pos+3] = JP_PHONEME_MAP.get(lyric, "a  ").encode('ascii')[:3]
        new_enot_list += this_enot

    # 3. Assembly and Size Updates
    # We need to update ECLS, ETRS, EDTS sizes as they contain ENOTs
    plgs_idx = data.find(b"PLGS")
    ecls_idx = data.find(b"ECLS")
    
    # Segment 1: Header to EVTS
    part1 = data[:evts_idx]
    # Segment 2: New EVTS chunk
    part2 = b"EVTS" + struct.pack("<I", len(evts_chunk_data)) + evts_chunk_data
    # Segment 3: PLGS to ECLS start (ECLS magic is at ecls_idx)
    part3 = data[plgs_idx : ecls_idx]
    
    # Segment 4: New ECLS/ETRS/EDTS part
    # Update ECLS count (offset 31 from ECLS magic in 007d4.bin)
    ecls_head = bytearray(data[ecls_idx : enot_idx])
    # Note count in ECLS is usually 1 byte at some offset. Let's find it.
    # In 007d4 it's at offset 31 (0x43E0+31 = 0x43FF which is just before ENOT)
    ecls_head[enot_idx - ecls_idx - 1] = len(notes)
    
    part4_enots = new_enot_list
    part5_tail = data[enot_idx + len(enot_template) : ]
    
    # Assemble display data part to calculate sizes
    display_part = ecls_head + part4_enots + part5_tail
    
    # Update ECLS size (at head+4)
    ecls_size = len(display_part) - 8
    ecls_head[4:8] = struct.pack("<I", ecls_size)
    
    # Join everything
    final = part1 + part2 + part3 + ecls_head + part4_enots + part5_tail
    
    # Final fix: Update parent chunk sizes (EDTS, ETRS)
    edts_idx = final.find(b"EDTS")
    etrs_idx = final.find(b"ETRS")
    
    # ETRS size (contains ECLS)
    etrs_size = len(final) - etrs_idx - 8 - 12 # Minus some tail
    # EDTS size
    edts_size = len(final) - edts_idx - 8
    
    # Update PPSF total size
    body_size = len(final) - 8
    final[4:8] = struct.pack("<I", body_size)

    with open(output_path, "wb") as f: f.write(final)
    print(f"Built v7: {output_path} with {len(notes)} notes.")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        curr = 960
        for i in range(0, len(args)-1, 2):
            melody.append((curr, int(args[i]), 480, args[i+1]))
            curr += 480
        build_ppsf_v7(melody, "built_v7.bin")
    else:
        # Do Re Mi Fa So
        m = [(960,60,480,"ど"),(1440,62,480,"れ"),(1920,64,480,"み"),(2400,65,480,"ふぁ"),(2880,67,480,"そ")]
        build_ppsf_v7(m, "built_v7.bin")
