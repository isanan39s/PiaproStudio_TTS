# PPSF (Piapro Studio Project File) 最終構造定義書

## 1. 階層ツリー (FourCC & Event Tree)

```text
[PPSF] (Size: File-8)
  ├── [INFO]
  ├── [PROJ]
  ├── [TRKS]
  ├── [EVTS] (Size: Payload)
  │     └── [0x08 Note Event] (独自形式 / 非FourCC)
  └── [EDTS] (Size: Payload)
        ├── [RECT]
        └── [ETRS]
              └── [ECLS]
                    └── [ENOT] (Editor Note Container)
                          ├── [1] + [VSQS] (Lyrics & Phoneme)
                          ├── [5] + [VSQA] (Attributes)
                          └── [3/4/??] + [VSQA] (Other Attributes)
```

---

## 2. 各コンポーネントの詳細

### 2.1 EVTS 内部：0x08 Note Event (音響用)
FourCC ではない独自イベント形式。

| オフセット | サイズ | 名称 | 内容 (JSON実値例) |
| :--- | :--- | :--- | :--- |
| +0 | 1 | **Magic** | `0x08` |
| +1 | 2 | **Size** | `42, 0` (0x002A = 42バイト) |
| +3 | 4 | **Tick** | `192, 3, 0, 0` (960) |
| +7 | 1 | **Pitch** | `60` (C3) |
| +8 | 4 | **Duration** | `192, 3, 0, 0` (960) |
| +12 | 4 | **Unk** | `64, 8, 0, 0` |
| +16 | 4 | **Velocity/Flags** | `50, 50, 127, 3` (Vel=50, 50, 127, Flag=3) |
| +20 | 3 | **Lyric** | `227, 131, 137` ("ど") |
| +23 | 1 | **Separator** | `0x00` |
| +24 | 1 | **PhonemeLen** | `3` |
| +25 | 3 | **Phoneme** | `100, 32, 111` ("d o") |
| +28 | 2 | **Separator** | `0x00, 0x00` |
| +30 | 1 | **SuffixLen** | `6` |
| +31 | 6 | **Suffix** | `"normal"` |

---

### 2.2 ENOT 内部構造 (表示用コンテナ)
FourCC チャンクを ID プレフィックス付きで内包する。

| 相対位置 | サイズ | 名称 | 内容 |
| :--- | :--- | :--- | :--- |
| +0x00 | 4 | **Magic** | `"ENOT"` |
| +0x04 | 4 | **Size** | `174, 0, 0, 0` (0xAE = 174バイト) |
| +0x08 | 4 | **Tick** | `192, 3, 0, 0` (960) |
| +0x0C | 4 | **UnkTick?** | `192, 3, 0, 0` |
| +0x10 | 4 | **Duration** | `192, 3, 0, 0` (960) |
| +0x14 | 10 | **Flags/Pad** | `0, 0, 0, 0, 0, 0, 0, 0, 255, 255` |

#### 2.2.1 内部サブチャンク：VSQS (歌詞・発音)
| 相対位置 | サイズ | 名称 | 内容 |
| :--- | :--- | :--- | :--- |
| +0 | 1 | **ID** | `0x01` |
| +1 | 4 | **Magic** | `"VSQS"` (86, 83, 81, 83) |
| +5 | 4 | **Size** | `22, 0, 0, 0` |
| +9 | 1 | **LyricLen** | `3` |
| +10 | 3 | **Lyric** | `"ど"` (UTF-8) |
| +13 | 1 | **PhonemeLen** | `3` |
| +14 | 3 | **Phoneme** | `"d o"` (ASCII) |

#### 2.2.2 内部サブチャンク：VSQA (属性)
| 相対位置 | サイズ | 名称 | 内容 |
| :--- | :--- | :--- | :--- |
| +0 | 1 | **ID** | `0x05` (または 0x03, 0x04 等) |
| +1 | 4 | **Magic** | `"VSQA"` |
| +5 | 4 | **Size** | `12, 0, 0, 0` |
| +9〜 | 可変 | **Value** | `"2, -1"` 等の属性値 |

---

## 3. 再構築（リビルド）のアルゴリズム

1.  **Bottom-Up Size Calc**:
    - 歌詞を変える → `VSQS` の Size 更新。
    - `VSQS` を含む `ENOT` の Size 更新。
    - `ENOT` を含む `ECLS` -> `ETRS` -> `EDTS` -> `PPSF` の Size を連鎖的に加算。
2.  **Sync Rule**:
    - `EVTS` 内部の `0x08` イベントの `Tick`, `Pitch`, `Duration` を、対応する `ENOT` の先頭フィールドと完全に一致させる。
3.  **FourCC Header**:
    - 各チャンクのサイズフィールドは **Payload (データ本体) の長さのみ** を含み、Magic と Size 自体の 8 バイトは除外する。
    - ただし `PPSF` 自体のサイズは、その直後のバージョン情報等も Payload に含めて計算する。
