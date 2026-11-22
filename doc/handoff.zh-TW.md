# 量測 git handoff

[English](handoff.md) | 繁體中文

Loci 在實作之前先做了量測，對象是一個產品 repository（perxona，Python backend 加 TypeScript
frontend，用 Claude Code 開發）的 session。本頁記錄量了什麼、怎麼量，以及這些結果不能說明什麼。

## 1. token 花在哪裡

這個 repository 所有的主 session，依 token 類型和每一輪做的事來分：

| 量測項目                           | 數值                 |
| ---------------------------------- | -------------------- |
| cache read 佔所有 token 的比例     | 97.8%                |
| 每輪帶著的 context                 | 125k 到 270k token   |
| 每個 feature branch／bugfix branch | 約 78M／約 24M token |
| 佔最多的一類                       | 探索，21% 到 32%     |
| 改程式碼                           | 不到 4%              |

cache read 的價格是 input token 的十分之一，但每一輪都要為整個帶著的 context 付一次。
要讓每輪讀得少，就要帶得少：開始新任務時，不要帶著上一個任務。

## 2. 離線重放

`loci replay` 會把過去每個 session 都套用一次規則，假設使用者在每次建議時都執行 `/clear`，
並用模型推算之後的每一輪：

- 之後每一輪的 context，等於這個 session 本身的底量，加上 handoff，再加上建議之後長出來的部分；
- handoff 之後的第一輪，會把新的 prefix 寫進 cache；
- session 自己 compact 之後，就不再推算。

cost units 照價目表加權：input 1、cache write 1.25、cache read 0.1、output 5。

磁碟上還留著的 20 個 session（3,412 輪），handoff 以 300 token 計算：

| 用途    | token：現在 → 用 Loci   | cost units：現在 → 用 Loci |
| ------- | ----------------------- | -------------------------- |
| feature | 509.5M → 280.1M（−45%） | 61.2M → 38.7M（−37%）      |
| bugfix  | 158.5M → 108.5M（−32%） | 19.8M → 15.0M（−24%）      |
| other   | 114.4M → 94.1M（−18%）  | 15.9M → 14.0M（−12%）      |
| 合計    | 782.3M → 482.7M（−38%） | 96.8M → 67.7M（−30%）      |

規則判斷了 57 次 branch 切換：15 次建議、11 次 revisit、7 次 long-lived branch、24 次低於門檻。
先前每次切換都建議的規則觸發得比較多，多半是 rebase 或 review 時回到原本的 branch，省下的卻沒多多少；
只看新 branch 的規則把這些都排除了。

重放是模型推算，不是實際執行：它看不出清空後的 session 是否要多花幾輪才能重新進入狀況。

## 3. 實際 A/B 重跑

為了驗證模型，挑了兩個真實任務，由 agent 以 headless 方式重新執行，兩種方式各跑兩次：

- **沒有 Loci：** 從切換 branch 之前接續原本的 session，帶著全部的 context。
- **有 Loci：** 全新的 session，只拿到 Loci 的 handoff。

兩組拿到相同的 prompt（issue 內文和貼在上面的決定）、相同的起點 commit、相同的模型，
上限是 80 或 120 輪。push、`gh`、網路和套件安裝都被擋掉；每次執行後都檢查過沒有 remote 指令，
真正的 repository 也沒有被改動。

### #8119，backend 修正；切換時 context 約 80k

| 次數 | token：沒有 → 有 Loci | 費用                  | 時間        |
| ---- | --------------------- | --------------------- | ----------- |
| 1    | 4.66M → 4.06M（−13%） | $2.52 → $2.27（−10%） | 688s → 593s |
| 2    | 5.68M → 5.10M（−10%） | $2.83 → $2.61（−8%）  | 777s → 708s |

兩組都通過自己的測試和真實修正的 regression test，並在真實修正的 110 個 importer 測試中失敗同樣
5 個：那些是後來才決定的細節，issue 裡沒有寫。

### #8386 和 #8388，frontend 加 backend 修正；切換時 context 約 298k

| 次數 | token：沒有 → 有 Loci   | 費用                   | 最大 context |
| ---- | ----------------------- | ---------------------- | ------------ |
| 1    | 48.29M → 15.94M（−67%） | $15.29 → $6.55（−57%） | 496k → 245k  |
| 2    | 27.03M → 14.86M（−45%） | $10.88 → $6.20（−43%） | 417k → 226k  |

平均：token −59%，費用 −51%，花的時間相同。第 1 次沒有 Loci 的那組撞到 120 輪上限，
所以它的費用是低估值。

| 檢查                               | 沒有 Loci     | 有 Loci       |
| ---------------------------------- | ------------- | ------------- |
| 自己的 backend 測試                | 808、808 通過 | 808、810 通過 |
| 自己的 frontend 測試               | 全部通過      | 全部通過      |
| 真實 pull request 的 backend 測試  | 44/44、44/44  | 44/44、44/44  |
| 和真實 pull request 改到相同的檔案 | 28/31、30/31  | 27/29、27/29  |

貼在 issue 上的每一項決定，都在四次執行的 diff 裡逐一核對過，全部都有做到。真實 pull request
的 frontend 測試在每一組都有部分失敗，因為它們綁定了它自己取的 helper 和 props 名稱。唯一的差異：
有 Loci 的兩次照專案自己的 skill 規定，在 translations 的 pull request merge 之前不動 submodule 的
指標；沒有 Loci 的兩次則把它指到還沒 merge 的本機 commit。

## 4. 這些結果不能說明什麼

- 兩個案例，每個各跑兩次：方向很清楚，幅度還不確定。
- 兩組的 prompt 都附上了 issue 內文。實際使用時，清空後的 session 要自己去 tracker 讀，
  會多花幾輪；handoff 會指出 issue，就是為了這個。
- 使用者會不會照著建議做。這要用 `loci stats` 從實際使用中量測。
