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

def create_note_binary(pos, pitch, dur, lyric):
    phoneme = JP_PHONEME_MAP.get(lyric, "a")
    lyric_b = lyric.encode('utf-8')
    phoneme_b = phoneme.encode('utf-8')
    
    # 1. Header (16 bytes)
    header = struct.pack("<ibi", pos, pitch, dur) + b"\x00"*4
    
    # 2. Lyric/Phoneme part
    lyric_part = struct.pack("B", len(lyric_b)) + lyric_b + b"\x00"
    phoneme_part = struct.pack("B", len(phoneme_b)) + phoneme_b + b"\x00"
    
    # 3. Tail (Fixed style)
    tail = b"\x06normal\x00\x00\x00\x00\x00\x00\x00\x00"
    
    body = header + lyric_part + phoneme_part + tail
    return b"\x08" + struct.pack("<H", len(body)) + body

def patch_ppsf(notes, output_path):
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
    head_to_first_note = data[evts_idx+8 : note_start]
    
    new_events_body = head_to_first_note
    for pos, pitch, dur, lyric in notes:
        new_events_body += create_note_binary(pos, pitch, dur, lyric)

    # PrefixedArray prefix (1 byte count for simpler logic if it fits in 1 byte)
    # Actually, the template uses a specific encoder for prefix.
    # For a small number of notes, we can mimic the template prefix.
    prefix = b"\x01" # Simulating the Select(PrefixedArray(Byte, subcon))
    
    new_chunk_data = prefix + new_events_body
    new_chunk = b"EVTS" + struct.pack("<I", len(new_chunk_data)) + new_chunk_data
    
    plgs_idx = data.find(b"PLGS")
    final_data = data[:evts_idx] + new_chunk + data[plgs_idx:]
    
    # Update total size
    body_size = len(final_data) - 8
    final_data = final_data[:4] + struct.pack("<I", body_size) + final_data[8:]

    with open(output_path, "wb") as f:
        f.write(final_data)
    print(f"Generated {output_path} with {len(notes)} notes.")

if __name__ == "__main__":
    # Default melody if no arguments provided
    melody = [
        (960,  60, 480, "ど"),
        (1440, 62, 480, "れ"),
        (1920, 64, 480, "み"),
        (2400, 65, 480, "ふぁ"),
        (2880, 67, 480, "そ")
    ]
    
    if len(sys.argv) > 1:
        args = sys.argv[1:]
        melody = []
        current_pos = 960  # Start at beat 2
        duration = 480     # Default to quarter note (480 ticks)
        
        # Process pairs of (pitch, lyric)
        # Usage: python3 build_ppsf_v2.py 60 こ 60 ん 62 に 62 ち 64 わ
        for i in range(0, len(args) - 1, 2):
            try:
                pitch = int(args[i])
                lyric = args[i+1]
                melody.append((current_pos, pitch, duration, lyric))
                current_pos += duration # Auto-sequence
            except ValueError:
                print(f"Skipping invalid pitch: {args[i]}")

    if not melody:
        print("Usage: python3 build_ppsf_v2.py <pitch1> <lyric1> <pitch2> <lyric2> ...")
        sys.exit(1)

    patch_ppsf(melody, "built_melody.bin")
