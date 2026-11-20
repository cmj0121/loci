# Loci 調查

[English](survey.md) | 繁體中文

gate 量測已經證明 Loci 不適合當找 code 的工具（見[量測](measurement.zh-TW.md)）。之後，我們用同一個問題
檢驗 agent 可能用上的其他每一種記憶：在真實的團隊 repository 上，它能不能省 token？每一節陳述一個結論，
背後的數字在 `doc/results/`。所有量測都在同一個大型 repository 上進行，由多人和 coding agent 共同開發；
本文不寫出它的名稱。

## 1. agent 很少重複探索，所以 session 記憶沒有什麼可以省

探索占了 agent 讀取內容的大部分，但幾乎都是為了當下任務而第一次讀。在之後的 session 重讀沒改過的檔案，
只占探索量約 3%；而從過去 session 抓下來的記憶，多半是工具環境相關的雜訊。過去 session 的記憶能省的
空間很小。[結果](results/survey-1-exploration.md)
