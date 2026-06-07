
# * LibreSVIPは[こちら](https://github.com/SoulMelody/LibreSVIP)をダウンロードし、改造したものです わたしは作っていません *



# Piapro Studio PPSF 生成パイプライン：運用要約書

## 1. システム構成図
本システムは、LibreSVIP の生成ロジックと、Go で実装された「外科手術的パッチャー」を組み合わせ、高速かつ正確な PPSF 生成を実現します。

```text
[Go Client] -- HTTP JSON --> [Python FastAPI Server]
                                ├── LibreSVIP Generator (via LibreSVIPppsf)
                                └── Surgical Patcher (Lyric/Pitch/Sync)
```

## 2. ディレクトリ構成と役割
- **`LibreSVIP-main` (公式ベース)**: LibreSVIP のコアモデル（Project, Note, Track 等）をここからインポートします。
- **`LibreSVIPppsf` (カスタムレイヤー)**: 
  - 本プロジェクトの核心部分。
  - 公式ライブラリの `PiaproStudioLegacyGenerator` をカスタマイズし、PPSF の外科手術的パッチングに特化させています。
  - API サーバーはこのフォルダ内のスクリプトを優先的に参照し、独自の生成・パッチロジックを適用します。

## 3. 環境構築手順

LibreSVIP の依存関係を保持するために、専用の仮想環境を使用します。

```bash
# LibreSVIP ソースディレクトリへ移動
cd LibreSVIP-main

# 仮想環境の作成と依存インストール
python3 -m venv venv
./venv/bin/pip install -e . fastapi uvicorn

# APIサーバーの起動 (バックグラウンド)
./venv/bin/python ../ppsf_api_server.py &
```

## 4. 主要コンポーネント

### 4.1 Python API サーバー (`ppsf_api_server.py`)
- **生成**: LibreSVIP のモデル生成機能を利用し、PPSF の基礎構造をメモリ上で構築。
- **Surgical Patcher**: `patch_binary` 関数および `patchStringsSurgical` を介し、指定したオフセットに対して Pitch, Tick, Duration, Lyric, Phoneme をピンポイントで上書き。これにより歌詞の「デフォルト値（"la"）」を完全に排除。

### 4.2 Go クライアント (`call_ppsf_api.go`)
- API に対してメロディデータを送信し、返却された PPSF バイナリをファイルとして保存。

## 5. 解決した主要課題
- **I/O エラー**: ENOT チャンク内部の Duration 同期不良とサイズ不整合を解消。
- **0分音符**: Pitch と Duration のオフセットを 1 バイト単位で再マッピングし、破損を防止。
- **歌詞の欠落**: LibreSVIP の生成物に依存せず、ターゲットとなる Hiragana/Phoneme を直接バイナリパッチすることで解決。

## 6. 今後の拡張
- **MusicXML/MIDI 対応**: LibreSVIP の CLI を活用して入力フォーマットを拡張可能。
- **全自動調教**: 音符ごとの発音記号変換テーブルを拡充し、歌詞入力だけで発音まで完結させる自動化が可能です。
