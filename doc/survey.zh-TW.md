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

## 3. review 一再重複的，是沒有人寫下來的團隊慣例

在繁忙的團隊 repository 裡，看得見的成本是 review 的往返，而不是找 code：大部分 PR 至少被退回一次。
三分之一的 review 留言，重複的是已經在三個以上 PR 提過的規則，而且這些規則大多沒有寫進 repository
自己的 agent 規則。git 保留了其中一部分知識：39% 的 review 留言，在 follow-up commit 的 message 裡
寫出了那條規則。[結果](results/survey-3-reviews.md)

## 4. push 前的規則檢查多抓到一些問題，但省不到 review 輪數

從 git 挖出規則，再依改動碰到的路徑挑選，抓到的問題大約是單純 push 前檢查的兩倍；把全部規則當純文字
塞進去，反而比完全不給規則還差。但一輪 review 通常包含好幾個要求，抓到其中一個，很少能讓整輪消失：
這個檢查避免了 2.7% 的輪數，不給規則時是 1.8%，而且花掉的 token 比省下的多。挖出來的規則大多也不會
再出現：96% 只出現在一個 PR。[結果](results/survey-4-precheck.md)

## 5. Loci 保留什麼：確定性的慣例檢查，以及這套評估方法

所有送給 agent 的記憶都沒有回本，而從歷史學 review 慣例，在 2026 年已經是一個產品類別。仍值得保留的
很少：一是挖出少數會一再重複的慣例，把每一條變成寫下來的規則，或每次改動都會執行的確定性檢查；二是
評估方法本身（時間 cutoff、不給規則和純文字兩種對照組、盲判、以 review 輪數計算、實測每輪成本），
現有工具只涵蓋其中一部分。Loci 不再作為產品繼續開發。[結果](results/survey-5-tools.md)
