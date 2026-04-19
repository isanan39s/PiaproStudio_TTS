import sys
import os
import struct

# LibreSVIP root directory path
current_dir = os.getcwd()
libresvip_root = os.path.join(current_dir, "mikusqe/LibreSVIP-main")
sys.path.append(libresvip_root)

try:
    from libresvip.plugins.ppsf.legacy_model import PpsfLegacyProject
    from construct import Container
except ImportError:
    print("Error: Required libraries not found.")
    print("Please run: pip install construct construct-typed")
    sys.exit(1)

# Japanese Lyric to Phoneme mapping (extended based on PDF manual)
JP_PHONEME_MAP = {
    "あ": "a", "い": "i", "う": "M", "え": "e", "お": "o",
    "か": "k a", "き": "k' i", "く": "k M", "け": "k e", "こ": "k o",
    "さ": "s a", "し": "S i", "す": "s M", "せ": "s e", "そ": "s o",
    "た": "t a", "ち": "tS i", "つ": "ts M", "て": "t e", "と": "t o",
    "な": "n a", "に": "J i", "ぬ": "n M", "ね": "n e", "の": "n o",
    "は": "h a", "ひ": "C i", "ふ": "p\ M", "へ": "h e", "ほ": "h o",
    "ま": "m a", "み": "m' i", "む": "m M", "め": "m e", "も": "m o",
    "や": "j a", "ゆ": "j M", "よ": "j o",
    "ら": "4 a", "り": "4' i", "る": "4 M", "れ": "4 e", "ろ": "4 o",
    "わ": "w a", "を": "o", "ん": "n",
    "ど": "d o", "れ": "4 e", "み": "m' i", "ふぁ": "p\ a", "そ": "s o", "ら": "4 a", "し": "S i"
}

def create_v3_note_event(pos, pitch, dur, lyric):
    phoneme = JP_PHONEME_MAP.get(lyric, "a")
    
    # Numerical header part (16 bytes)
    # pos (4), pitch (1), padding (3), duration (4), unknown (4)
    header_data = struct.pack("<ibi", pos, pitch, dur)
    full_header = header_data + b"\x00\x00\x00\x00" 
    
    # Lyric info
    lyric_bytes = lyric.encode('utf-8')
    phoneme_bytes = phoneme.encode('utf-8')
    
    lyric_part = struct.pack("B", len(lyric_bytes)) + lyric_bytes + b"\x00"
    phoneme_part = struct.pack("B", len(phoneme_bytes)) + phoneme_bytes + b"\x00"
    
    # Extra flags (template style)
    tail_part = b"\x06normal\x00\x00\x00\x00\x00\x00\x00\x00"
    
    data = full_header + lyric_part + phoneme_part + tail_part
    
    return Container(magic="Vocaloid3NoteEvent", size=len(data), data=data)

def build_ppsf(notes, output_path):
    template_path = "raw_state_005c3.bin"
    if not os.path.exists(template_path):
        print(f"Error: Template {template_path} not found.")
        return

    with open(template_path, "rb") as f:
        project = PpsfLegacyProject.parse(f.read())

    # Navigate to the Events chunk
    for chunk in project.body.chunks:
        if chunk.magic == "Events":
            # events[0] is the inner list due to ppsf_prefixed_array implementation
            original_events = chunk.data.events[0]
            new_event_list = []
            
            # Keep non-note events (MidiEvents for tempo/key etc.)
            for event in original_events:
                if event.magic != "Vocaloid3NoteEvent":
                    new_event_list.append(event)
            
            # Add our new notes
            for pos, pitch, dur, lyric in notes:
                new_event_list.append(create_v3_note_event(pos, pitch, dur, lyric))
            
            # Re-assign
            chunk.data.events[0] = new_event_list

    # Build binary
    try:
        new_binary = PpsfLegacyProject.build(project)
        with open(output_path, "wb") as f:
            f.write(new_binary)
        print(f"Project successfully built to {output_path}")
    except Exception as e:
        print(f"Error building project: {e}")

if __name__ == "__main__":
    # Test Scale
    test_scale = [
        (960,  60, 480, "ど"),
        (1440, 62, 480, "れ"),
        (1920, 64, 480, "み"),
        (2400, 65, 480, "ふぁ"),
        (2880, 67, 480, "そ"),
        (3360, 69, 480, "ら"),
        (3840, 71, 480, "し"),
        (4320, 72, 960, "ど"),
    ]
    build_ppsf(test_scale, "custom_melody.bin")
