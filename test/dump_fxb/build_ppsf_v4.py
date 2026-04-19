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

def create_note_binary(pos, pitch, dur, lyric, is_first=False):
    phoneme = JP_PHONEME_MAP.get(lyric, "a")
    lyric_b = lyric.encode('utf-8')
    phoneme_b = phoneme.encode('utf-8')
    
    # 1. Header (Exactly 16 bytes)
    # [Pos:4][Pitch:1][Dur:4][Attrs:7]
    attrs = b"\x40\x08\x00\x00\x32\x32\x7f"
    header = struct.pack("<iBi", pos, pitch, dur) + attrs
    
    # 2. Strings
    lyric_part = struct.pack("B", len(lyric_b)) + lyric_b + b"\x00"
    phoneme_part = struct.pack("B", len(phoneme_b)) + phoneme_b + b"\x00"
    style_part = b"\x00\x06normal\x00" # Extra null before 06normal
    
    # 3. Tail
    if is_first:
        tail = b"\x00" * 7
    else:
        # Note 2+ uses 19 bytes extended data
        tail = b"\x00\x7a\x02\x01\x40\x00\x00\x00\x00\x00\x01\x32\x00\x00\x00\x00\x00\x00\x00"
    
    body = header + lyric_part + phoneme_part + style_part + tail
    # Event format: [Magic: 0x08][Size: 2bytes LE][Data]
    return b"\x08" + struct.pack("<H", len(body)) + body

def build_ppsf(notes, output_path):
    template_path = "raw_state_005c3.bin"
    if not os.path.exists(template_path):
        print(f"Error: Template {template_path} not found.")
        return

    with open(template_path, "rb") as f:
        data = bytearray(f.read())

    evts_idx = data.find(b"EVTS")
    if evts_idx == -1:
        print("EVTS chunk not found")
        return

    note_start = data.find(b"\x08", evts_idx)
    head_to_first_note = data[evts_idx+9 : note_start]
    
    new_events_body = b""
    for i, (pos, pitch, dur, lyric) in enumerate(notes):
        new_events_body += create_note_binary(pos, pitch, dur, lyric, is_first=(i==0))

    # Reconstruct EVTS chunk (with trailing zero)
    new_chunk_data = struct.pack("B", len(notes)) + head_to_first_note + new_events_body + b"\x00"
    new_chunk = b"EVTS" + struct.pack("<I", len(new_chunk_data)) + new_chunk_data
    
    plgs_idx = data.find(b"PLGS")
    final_data = data[:evts_idx] + new_chunk + data[plgs_idx:]
    
    # Update total size
    body_size = len(final_data) - 8
    final_data[4:8] = struct.pack("<I", body_size)

    with open(output_path, "wb") as f:
        f.write(final_data)
    print(f"Generated {output_path} with {len(notes)} notes.")

if __name__ == "__main__":
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        current_pos = 960
        duration = 480
        for i in range(0, len(args) - 1, 2):
            try:
                pitch = int(args[i])
                lyric = args[i+1]
                melody.append((current_pos, pitch, duration, lyric))
                current_pos += duration
            except: pass
        build_ppsf(melody, "built_v4.bin")
    else:
        # Default Do-Re-Mi
        melody = [(960, 60, 480, "ど"), (1440, 62, 480, "れ"), (1920, 64, 480, "み")]
        build_ppsf(melody, "built_v4.bin")
