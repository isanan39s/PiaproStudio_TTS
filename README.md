# PiaproStudio_TTS
ミクさんたちを喋らせたい

構成
⦁	piaplostudio v4x,vst2.4
⦁	voicevoxエンジン,httpapi
⦁	ホスト,上2つとwav出力

今回のホスト
1.	(汎用的な日本語入力(エンジンにリクエスト投げるだけ？))
2.	openjtalkからのクエリをppsf形式+@パラメータに変換

## thanks
このプログラムは以下のプロジェクト？を活用させていただいています
- [openjtalk](https://open-jtalk.sourceforge.net/) 漢字->かな&イントネーション取得に
- [LibreSVIP](https://github.com/SoulMelody/LibreSVIP) ppsfバイナリ生成に

勝手に使って何だってとこかもしれませんが ありがとうございます


---
https://github.com/maito1201/tinywindow/blob/main/main.go