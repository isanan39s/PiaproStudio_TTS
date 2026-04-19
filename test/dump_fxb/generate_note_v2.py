import sys
import os

# Mapping based on Piapro Studio Manual page 77-79
JP_PHONEME_MAP = {
    # Vowels
    "あ": "a", "い": "i", "う": "M", "え": "e", "お": "o",
    # K-line
    "か": "k a", "き": "k' i", "く": "k M", "け": "k e", "こ": "k o",
    # S-line
    "さ": "s a", "し": "S i", "す": "s M", "せ": "s e", "そ": "s o",
    # T-line
    "た": "t a", "ち": "tS i", "つ": "ts M", "て": "t e", "と": "t o",
    # N-line
    "な": "n a", "に": "J i", "ぬ": "n M", "ね": "n e", "の": "n o",
    # H-line
    "は": "h a", "ひ": "C i", "ふ": "p\ M", "へ": "h e", "ほ": "h o",
    # M-line
    "ま": "m a", "み": "m' i", "む": "m M", "め": "m e", "も": "m o",
    # Y-line
    "や": "j a", "ゆ": "j M", "よ": "j o",
    # R-line
    "ら": "4 a", "り": "4' i", "る": "4 M", "れ": "4 e", "ろ": "4 o",
    # W-line / N
    "わ": "w a", "を": "o", "ん": "n",
    # G-line (Dakuon)
    "が": "g a", "ぎ": "g' i", "ぐ": "g M", "げ": "g e", "ご": "g o",
    # Z-line
    "ざ": "dz a", "じ": "dz i", "ず": "dz M", "ぜ": "dz e", "ぞ": "dz o",
    # D-line
    "だ": "d a", "ぢ": "dz i", "づ": "dz M", "で": "d e", "ど": "d o",
    # B-line
    "ば": "b a", "び": "b' i", "ぶ": "b M", "べ": "b e", "ぼ": "b o",
    # P-line (Han-dakuon)
    "ぱ": "p a", "ぴ": "p' i", "ぷ": "p M", "ぺ": "p e", "ぽ": "p o",
}

def generate_note(pitch, lyric):
    template = "raw_state_007d4.bin"
    output = f"output_{pitch}_{lyric}.bin"
    
    if not os.path.exists(template):
        print(f"Error: Template {template} not found.")
        return

    phoneme = JP_PHONEME_MAP.get(lyric)
    if not phoneme:
        print(f"Error: Lyric '{lyric}' not found in mapping table.")
        return

    with open(template, "rb") as f:
        data = bytearray(f.read())

    # 1. Pitch offset (Hex: 0xBB6)
    data[0xBB6] = int(pitch)
    
    # 2. Lyric offset (Hex: 0xBC3)
    # UTF-8 for one character is usually 3 bytes
    lyric_bytes = lyric.encode('utf-8')
    if len(lyric_bytes) != 3:
        print(f"Warning: Lyric {lyric} is {len(lyric_bytes)} bytes. Structure might shift.")
    
    # Overwrite exactly 3 bytes at 0xBC3
    data[0xBC3:0xBC6] = lyric_bytes[:3]

    # 3. Phoneme length and string (Hex: 0xBC8)
    # 0xBC8 is the length byte, 0xBC9 is the start of the string
    phoneme_bytes = phoneme.encode('ascii')
    phoneme_len = len(phoneme_bytes)
    
    # Current template has 3 bytes space ("4 a")
    # To avoid shifting the rest of the file, we keep it to 3 bytes if possible
    # or just overwrite and hope the parser handles nulls
    data[0xBC8] = phoneme_len
    # Clear old phoneme (3 bytes)
    data[0xBC9:0xBCC] = b'\x00\x00\x00'
    # Write new phoneme
    data[0xBC9:0xBC9+phoneme_len] = phoneme_bytes

    with open(output, "wb") as f:
        f.write(data)
    
    print(f"Generated {output}")
    print(f"  Pitch: {pitch}")
    print(f"  Lyric: {lyric} ({lyric_bytes.hex(' ')})")
    print(f"  Phoneme: {phoneme} ({phoneme_bytes.hex(' ')})")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python3 generate_note_v2.py <midi_pitch> <lyric_char>")
    else:
        generate_note(sys.argv[1], sys.argv[2])
