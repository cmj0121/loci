# Loci 規格

[English](spec.md) | 繁體中文

本文件定義 Loci 的行為：記憶裡有什麼、什麼時候會改變，而不是它如何實作。
背後的想法請看 [README](../README.zh-TW.md)。

**狀態：未實作。** Benchmark 不支持 Loci，所以這份規格不會被實作；請看[結果](measurement.zh-TW.md#3-結果)。

## 1. 範圍

- Loci 是個人使用的工具。一個本機 service 服務同一台機器上的所有專案。
- Loci 只認得已經 commit 的程式碼。尚未 commit 的修改由 agent 自己記住，等 commit 之後才會進入記憶。
- Loci 不呼叫 LLM。對程式碼的理解由 agent 的 LLM 撰寫，再交給 Loci 保存。

## 2. Code memory

記憶分成兩層。

| 層級    | 內容                             | 由誰寫入           |
| ------- | -------------------------------- | ------------------ |
| Symbols | 每個 function 的名稱、檔案與位置 | Loci，透過 parsing |
| Notes   | 每個 function 一則 concept note  | LLM，透過 CLI      |

每個部分各有一件工作：

- **tree-sitter** 把每個 function 抽象成名稱、所在檔案與位置；
- **LLM** 依照 skill 的指引，為每個 function 寫一則 note；
- **`loci` CLI** 是寫入與更新 note 的途徑。

Symbols 是以 deterministic 的方式抽取的，所以隨時可以從 repository 重建。
Notes 不行：它是記憶裡唯一不是從程式碼推導出來的部分。

## 3. 沿著歷史追蹤 function

Note 屬於 function 本身，而不是它的名稱或位置。因此 Loci 會從一個 commit 到下一個 commit
追蹤每個 function。

| Commit 裡發生的變化         | Loci 做的事                                        |
| --------------------------- | -------------------------------------------------- |
| function 改名               | 偵測為改名——同一個 function、新的名稱；note 保留   |
| function 搬到別的檔案或位置 | 保留 note，只更新位置                              |
| 實作或概念改變              | 把 note 標示為「待更新」，直到 LLM 重寫            |
| function 新增               | 一個新的 function，還沒有 note                     |
| function 移除               | 從該 commit 起，這個 function 與它的 note 都不存在 |

改名的判斷依據是：function 的內容不變，而名稱變了。如果同一個 commit 裡既改名又大幅改寫，
就無法與「移除一個 function、新增另一個」區分，會被當成後者處理。

## 4. Notes 與 revision

記憶忠於 revision：

- 同一個 function 在不同的 commit 可以有不同的 note；
- 某個 commit 裡不存在的 function，在那裡就沒有 note；
- 在任何 revision 上查詢，得到的都是屬於那裡的程式碼的 note。

Note 有兩種狀態。

| 狀態   | 意思                             | 查詢回傳的內容            |
| ------ | -------------------------------- | ------------------------- |
| 最新   | 針對 function 目前這個版本所寫   | 該則 note                 |
| 待更新 | note 寫好之後，function 又改變了 | 先前的 note，並帶著此標示 |

待更新的 note 仍然會回傳，讓 agent 可以把先前的概念當成線索。Skill 會告訴 agent 看到這個標示時
該怎麼做：先讀程式碼，再透過 `loci` CLI 寫下新的 note。寫入之後，該 commit 的 note 就成為最新；
先前的 commit 則保有它原本的 note。

## 5. 建立記憶

Git hooks 就是觸發點。不會因為有人查詢而建立任何東西。

| Git 操作                 | Loci 做的事                                       |
| ------------------------ | ------------------------------------------------- |
| `commit`                 | 為新的 commit 建立記憶                            |
| `checkout` / `switch`    | 切換到該 revision 的記憶                          |
| `merge` / `rebase`       | 跟上新的歷史，重複使用已經知道的內容              |
| 刪除 branch、丟棄 commit | 清除已經沒有任何 revision 用得到的部分（第 7 節） |

建立的規則：

1. **立刻開始。** Hook 一觸發就開始建立。
2. **在背景執行。** Hook 會立即返回。Loci 絕不會阻擋 git 操作，也不會讓它失敗。
3. **以 commit 為單位。** 一次建立涵蓋一個 commit，並重複使用未變動內容的既有記憶。
4. **一次發布。** 要等該 commit 完整建好，專案的記憶才會切換到新的 commit。
5. **讀取的人不必等待，也不會看到建到一半的記憶。** 建立期間，查詢由上一份已完成的記憶回答。
6. **每個答案都標明它的 commit。** Agent 永遠能分辨答案描述的是哪個 commit，
   也就知道自己讀到的是不是較早的記憶。

相同的內容只會記一次，不論有多少 branch 與 commit 包含它。切換到已經建好的 revision
只是一次查詢，而不是重建。

## 6. 漏掉的 hook 與重建

Hook 有可能漏掉：service 沒有在執行，或是 hook 連不上它。Loci 不會隱藏這個缺口。

- **啟動時。** Service 啟動時，會為每個專案目前的 commit 補建缺少的記憶。
- **查詢時。** 查詢到還沒有記憶的 commit 時，會明確告知，並改為提供最近一份已完成的記憶。
  查詢本身不會觸發建立。
- **手動要求。** 使用者隨時可以重建，方式有兩種：透過 command line，或透過 skill。

## 7. 清除

刪除 branch 或丟棄 commit 之後，剩下的 revision 都用不到的記憶會被移除。
Repository 的設定也可以限制要保留多少記憶（第 8 節）。

## 8. 設定

設定分成兩層。

| 層級   | 位置                       | 適用範圍          |
| ------ | -------------------------- | ----------------- |
| Global | service 自己的 setup       | 所有專案          |
| Local  | repository 裡的 `.loci` 檔 | 只有該 repository |

- `.loci` 可以覆寫 global setup 的一部分或全部；它沒有提到的項目，就沿用 global setup。
- 設定項目至少包含：repository 的記憶存放在哪裡，以及要保留多少。
- Loci 只會讀取 `.loci`。要不要 commit 它，由使用者決定。

## 9. Agents

在同一個專案上工作的每個 agent，讀寫的是同一份記憶。

- **Skill。** 告訴 agent 什麼時候該問 Loci 而不是做全文搜尋、如何為 function 寫 note、
  遇到待更新的 note 該怎麼做，以及如何要求重建。
- **Prompt hook。** `UserPromptSubmit` hook 在每次 prompt 時執行 `loci suggest`，注入可能相關的 function。
  沒有符合的就不注入任何東西。
- **CLI。** Agent 執行的 `loci` 指令：用 `find`、`show`、`search` 與 `outline` 查詢 function 並讀取它的 note，
  以及一個寫入或更新 note 的指令。

沒有 MCP server。

答案要小：一個位置加上一兩行，而不是整個檔案。

## 待定事項

尚未決定：

- 寫入 note 的 `loci` 指令，以及每個指令答案的格式；
- `.loci` 檔接受哪些設定，以及它的格式；
- 「保留多少」是以什麼計算——revision 數量、時間或容量——以及先丟棄哪些；
- 清除時如何對待 notes，因為 notes 無法重建；
- 先支援哪些程式語言的 parsing；
- 除了 function 之外（type、method、module），是否也有 note。
