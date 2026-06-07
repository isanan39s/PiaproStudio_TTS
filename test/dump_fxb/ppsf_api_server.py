import sys
import os
import json
import struct
import uvicorn
from fastapi import FastAPI, Request, Response
from libresvip.plugins.ppsf.piapro_studio_legacy_generator import PiaproStudioLegacyGenerator
from libresvip.plugins.ppsf.options import OutputOptions
from libresvip.model.base import Project, SingingTrack, Note, SongTempo, TimeSignature

# LibreSVIPのパスを通す
sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "LibreSVIP-main"))

app = FastAPI()

def patch_binary(data, notes):
    """
    EVTSとENOTを外科手術的にパッチする
    """
    # ターゲット: LibreSVIPのデフォルト歌詞 (UTF-8)
    targets = [b"\xe3\x82\x89"] # 「ら」
    
    # 1. EVTS Patch
    evts_idx = data.find(b"EVTS")
    if evts_idx != -1:
        p = evts_idx + 9
        for i, n in enumerate(notes):
            found = data.find(b"\x08", p)
            if found == -1: break
            pos = found
            # Patch Tick(+3), Pitch(+7), Duration(+8)
            struct.pack_into("<i", data, pos+3, n['tick'])
            data[pos+7] = n['pitch']
            struct.pack_into("<I", data, pos+8, n['dur'])
            
            # Patch Lyrics (3 bytes)
            lyric_b = n['lyric'].encode('utf-8')
            for t in targets:
                idx = data.find(t, pos, pos+60)
                if idx != -1:
                    data[idx:idx+3] = lyric_b[:3]
                    break
            p = pos + 40
            
    # 2. ENOT Patch
    p = 0
    for i, n in enumerate(notes):
        found = data.find(b"ENOT", p)
        if found == -1: break
        pos = found
        # Patch Tick(+8), Pitch(+14 is wrong, based on dump pitch is in VSQA), Duration(+16)
        struct.pack_into("<i", data, pos+8, n['tick'])
        struct.pack_into("<I", data, pos+16, n['dur'])
        p = pos + 20

    return data

@app.post("/generate")
async def generate(req: Request):
    body = await req.json()
    notes_req = body.get("notes", [])
    
    # LibreSVIPモデル構築
    notes = [Note(start_pos=n['tick'], length=n['dur'], key_number=n['pitch'], 
                  lyric=n.get('lyric', 'la'), pronunciation=n.get('phoneme', '4 a')) 
             for n in notes_req]
    
    project = Project(
        track_list=[SingingTrack(note_list=notes, title="Track 1")],
        song_tempo_list=[SongTempo(position=0, bpm=120.0)],
        time_signature_list=[TimeSignature(bar_index=0, numerator=4, denominator=4)]
    )
    
    # 生成
    generator = PiaproStudioLegacyGenerator(options=OutputOptions())
    binary_data = bytearray(generator.generate_project(project))
    
    # 最終パッチ
    final_binary = patch_binary(binary_data, notes_req)
    
    # 全体サイズ更新
    struct.pack_into("<I", final_binary, 4, len(final_binary)-8)
    
    return Response(content=bytes(final_binary), media_type="application/octet-stream")

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)
