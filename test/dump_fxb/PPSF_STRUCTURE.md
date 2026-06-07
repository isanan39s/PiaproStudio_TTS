# PPSF (Piapro Studio Project File) 完全構造定義書

## 1. 全体階層構造 (Root to Leaf)

```text
[PPSF] (Root)
  ├── [INFO] (Metadata)
  ├── [PROJ] (Project Settings: ボイスバンク名 "MIKU_V4X_..." 等を2回保持)
  ├── [TRNS] (Transport)
  ├── [CONF] (Configuration)
  ├── [DVCS] (Devices)
  ├── [TRKS] (Tracks Container)
  │     ├── [AETS] / [METS] / [VETS]
  │     └── [V3TS] (Vocaloid 3 Track Slot)
  │           └── [V3TK] (V3 Track)
  ├── [CLPS] (Clips Container)
  │     ├── [V3CL] (V3 Note Clip)
  │     └── [AMCL] (Audio Mixer Channel)
  ├── [EVTS] (Audio Events - 音響エンジン用)
  │     └── [0x08 Note Event] (音符列)
  ├── [PLGS] (Plugins Container)
  │     └── [EAPG] (Audio Mixer / EQ)
  └── [EDTS] (Editor Data - 表示用)
        ├── [RECT] (View Window)
        └── [ETRS] (Track Data)
              └── [ECLS] (Clip Data)
                    └── [ENOT] (Editor Note)
                          ├── [0x01] + [VSQS] (Lyrics)
                          └── [0x05] + [VSQA] (Attributes)
```

---

## 2. チャンク接続の特殊ルール (Prefix Bytes)

FourCC チャンクの直前には、多くの場合 **1バイトのプレフィックス** が存在します。

- **EVTS内部**: `0x08` (Note Event) の直前にはプレフィックスなし（0x08自体がType）。
- **ENOT内部**: `VSQS` の直前には `0x01`、`VSQA` の直前には `0x05` など。
- **TRKS/CLPS**: 子チャンクの直前に `0x00` や `0x01` が挟まるケースが多い。

---

## 3. 再構築における「サイズ」の定義

- **Leaf Chunk (ENOT, VSQS等)**: `Size` = Payloadの純粋な長さ。
- **Container Chunk (EDTS, TRKS等)**: `Size` = 子チャンク（Magic+Size+Prefix込）の合計長さ。
- **PPSF Root**: `Size` = 全データ長 - 8。

## 4. データ同期のチェックリスト

メロディを注入する際、最低限以下の 3 箇所を同期させる必要があります。

1.  **EVTS (0x08)**: 音響エンジン。`Tick`, `Pitch`, `Duration`, `Lyric`, `Phoneme` を書き換え。
2.  **ENOT**: 表示系。`Tick`, `Pitch`, `Duration` を EVTS と一致させる。
3.  **VSQS (inside ENOT)**: 表示用歌詞。`Lyric`, `Phoneme` を EVTS と一致させる。
