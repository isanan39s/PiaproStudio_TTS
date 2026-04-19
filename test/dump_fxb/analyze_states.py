import sys
import os
import struct

# LibreSVIPのモデルがあるパスをsys.pathに追加
current_dir = os.getcwd()
libresvip_root = os.path.join(current_dir, "mikusqe/LibreSVIP-main")
sys.path.append(libresvip_root)

try:
    from libresvip.plugins.ppsf.legacy_model import PpsfLegacyProject
except ImportError as e:
    print(f"Error: {e}")
    print("Dependencies might be missing. Attempting to run anyway...")

def analyze_file(file_path):
    print(f"\n--- Analyzing: {file_path} ---")
    if not os.path.exists(file_path):
        print(f"File not found: {file_path}")
        return

    try:
        with open(file_path, "rb") as f:
            content = f.read()
        
        # 1. パース実行
        project = PpsfLegacyProject.parse(content)
        
        # 2. Eventsチャンクを探す
        found_events = False
        for chunk in project.body.chunks:
            if chunk.magic == "Events":
                found_events = True
                print(f"Found Events Chunk (Size: {chunk.size})")
                # 3. ノートイベントを抽出
                for event_wrapper in chunk.data.events:
                    # Select(PrefixedArray) の構造により、データが入れ子になっている可能性があるため
                    # legacy_model.py の定義に合わせてアクセス
                    event = event_wrapper
                    if event.magic == "Vocaloid3NoteEvent":
                        # struct.unpack でデータを分解
                        # piapro_studio_legacy_parser.py によれば "<iii" (note_offset, pit, length)
                        try:
                            # event.data は bytes
                            pos, pit, length = struct.unpack_from("<iii", event.data)
                            # 12バイト目以降に歌詞情報があるはず
                            raw_tail = event.data[12:]
                            print(f"  Note: Pos={pos}, Pitch={pit}, Length={length}")
                            print(f"    Raw Tail (Hex): {raw_tail.hex(' ')}")
                            # 歌詞を文字列としてデコードを試みる（適当なオフセットで）
                            try:
                                # 最初の1バイトが長さのPascalStringの可能性がある
                                lyric_len = raw_tail[0]
                                lyric = raw_tail[1:1+lyric_len].decode('utf-8', errors='ignore')
                                print(f"    Possible Lyric: {lyric}")
                            except:
                                pass
                        except Exception as ex:
                            print(f"    Failed to unpack note data: {ex}")
        if not found_events:
            print("No Events chunk found in this file.")
            
    except Exception as e:
        print(f"Error parsing file: {e}")

# 比較したいファイル
files_to_compare = [
    "raw_state_005c3.bin",
    "raw_state_006c4.bin",
    "raw_state_007d4.bin",
    "raw_state_008d4あ.bin",
    "raw_state_009c3-b3ドレミファソラシ.bin"
]

if __name__ == "__main__":
    for f in files_to_compare:
        analyze_file(f)
