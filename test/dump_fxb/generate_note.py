import sys
import os

def generate_note(pitch, lyric):
    template = "raw_state_007d4.bin"
    output = f"output_{pitch}_{lyric}.bin"
    
    if not os.path.exists(template):
        print(f"Error: Template {template} not found.")
        return

    with open(template, "rb") as f:
        data = bytearray(f.read())

    # Pitch offset (Hex: 0xBB6)
    data[0xBB6] = int(pitch)
    
    # Lyric offset (Hex: 0xBC3) - Template has "ら" (e3 82 89)
    # Support for simple 3-byte lyrics
    lyric_bytes = lyric.encode('utf-8')
    if len(lyric_bytes) <= 3:
        # Pad or truncate to 3 bytes to maintain file structure integrity
        padded_lyric = lyric_bytes.ljust(3, b'\x00')
        data[0xBC3:0xBC6] = padded_lyric
    else:
        print("Error: For now, only lyrics up to 3 bytes (1 Japanese char) are supported to keep structure.")
        return

    with open(output, "wb") as f:
        f.write(data)
    
    print(f"Created {output}")
    print(f"Hex at pitch offset (0xBB6): {data[0xBB6:0xBB7].hex()}")
    print(f"Hex at lyric offset (0xBC3): {data[0xBC3:0xBC6].hex()}")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python3 generate_note.py <midi_pitch> <lyric_char>")
        print("Example: python3 generate_note.py 60 あ")
    else:
        generate_note(sys.argv[1], sys.argv[2])
