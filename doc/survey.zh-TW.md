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

## 2. 過去 fix 留下的 lesson 是對的，但很少能預防下一個錯

修正「先前 PR 已經 merge 的 code」的 fix，本身就是真正的 lesson，小模型能把大部分這類 fix 轉成正確、
可重用的規則。但下一個錯幾乎都是新的：一個 bug 被寫出來之前就已經存在的 lesson，只能預防約 0.75% 的
這類 bug，以及約 0.5% 的 PR 內修正輪次。agent 也幾乎不去挖歷史（只占 model input 的 3.45%），所以沒有
查詢成本可以替代。[結果](results/survey-2-fix-lessons.md)
