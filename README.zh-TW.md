# Loci

> 問你的程式碼怎麼運作，而不是某個名稱在哪裡。

[English](README.md) | 繁體中文

**狀態：已結束。** Loci 在實作之前先做了量測，結果並不划算：沒有任何版本能以可重現的方式，
在 token 與成本上都贏過單純的全文搜尋。Loci 不會被實作。請看[結果](doc/measurement.zh-TW.md#3-結果)。
本頁其餘內容描述的是被測試的假設。

Loci 回答關於某個行為的問題：它給出承載這個行為的 function，以及它們為什麼這樣寫——一個位置加上一行 note。
這份記憶留在本機，並且與 git 保持同步。

```text
> session 過期時，登入會發生什麼事？
loci  auth/session.rs:88  refresh_or_reject
      過期的 token 會先 refresh 一次，再失敗就拒絕（note @a1b2c3）
```

查名稱或關鍵字是全文搜尋的工作。即使在大的 repository 上，全文搜尋一兩次呼叫就找得到 function；
Loci 不打算在這裡贏過它。

## 為什麼需要

Agent 第一次進入一個 repository 時，很快就能找到名稱，卻找不到程式碼背後的理由。
為了弄懂原因，它讀進遠超過需要的內容，下一個 session 又重新弄懂一次。

Loci 把學到的東西留下來：

- **它怎麼運作？** 關於某個行為的問題，以承載它的 function 回答。
- **為什麼這樣寫？** 每個 function 一行 note，留給下一個 agent。
- **是哪個 revision？** 你目前 checkout 的那個 commit，不是過期的快照。

Benchmark 用 Loci 的替身測試了這一點。它唯一的收益——用 prompt hook 送上 notes 回答概念問題——沒有重現；
請看[結果](doc/measurement.zh-TW.md#3-結果)。

## 概念

```text
  ┌──────────────────┐  ┌──────────────────┐      ┌──────────────────────────┐
  │ git hooks        │  │ command line     │      │ agent                    │
  │ (per repository) │  │ e.g. rebuild     │      │ skill + prompt hook      │
  └────────┬─────────┘  └────────┬─────────┘      └──────────────────────────┘
           │                     │                             ▲
           │ git event           │ command                     │ ask, answer
           ▼                     ▼                             ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ Loci                                                                         │
│ ┌────────────────────────────────────────┐  ┌──────────────────────────────┐ │
│ │ event receiver                         │  │ loci CLI                     │ │
│ │ takes git events and commands,         │  │ find, show, search, outline; │ │
│ │ returns at once                        │  │ suggest; writes notes        │ │
│ └───────────────────┬────────────────────┘  └──────────────────────────────┘ │
│                     │                                        ▲               │
│                     ▼                                        │               │
│ ┌────────────────────────────────────────┐                   │               │
│ │ builder                                │                   │               │
│ │ parses each commit (tree-sitter),      │                   │ read, write   │
│ │ publishes it all at once               │                   │               │
│ └───────────────────┬────────────────────┘                   │               │
│                     │ publish                                │               │
│                     ▼                                        ▼               │
│ ┌──────────────────────────────────────────────────────────────────────────┐ │
│ │ data storage, per repository                                             │ │
│ │ ┌────────────────────┐  ┌────────────────────┐  ┌────────────────────┐   │ │
│ │ │ repo A             │  │ repo B             │  │ repo ...           │   │ │
│ │ │ symbols + notes,   │  │ symbols + notes,   │  │ symbols + notes,   │   │ │
│ │ │ per revision       │  │ per revision       │  │ per revision       │   │ │
│ │ └────────────────────┘  └────────────────────┘  └────────────────────┘   │ │
│ │ where and how much: global setup, overridden by each repo's .loci        │ │
│ └──────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────┘
```

- **Code memory。** 分成兩層：Loci 從原始碼 parse 出來的 symbols——每個 function 的名稱、
  檔案與位置——以及由 LLM 為每個 function 撰寫的 concept note。Loci 本身不呼叫 LLM。
- **需要時才寫 notes。** 只替被問到或最近改過的 function 寫 notes，而不是事先替每個 function 都寫。
  Gate 1 測試了這一點，結果並不划算。
- **Git-aware。** Git 操作就是觸發點。Commit 會建立它的記憶，checkout 會切換到該 revision
  的記憶，刪除 branch 或丟棄 commit 則會清除再也用不到的部分。
- **獨立的 service。** 一個個人、本機的 service 服務你所有的專案；有一份 global setup，
  每個 repository 可以用 `.loci` 檔覆寫。
- **由 agent 共享。** Prompt hook（`UserPromptSubmit`）執行 `loci suggest`，注入候選的 function。
  `loci` CLI（`find`、`show`、`search`、`outline`）與它的 skill 讓 agent 進一步查詢並寫回 note。
  沒有 MCP server。一個 agent 弄懂的事，下一個 agent 一開始就知道。

## 流程

從一次 commit 到 agent 拿到答案，中間發生的事：

```text
┌──────────────────────────────┐
│ git commit                   │
└───────────────┬──────────────┘
                │ hook fires
                ▼
┌──────────────────────────────┐
│ build the commit's memory    │   queries meanwhile get the
│ in the background            │   last completed memory
└───────────────┬──────────────┘
                │ build complete
                ▼
┌──────────────────────────────┐
│ publish all at once          │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ prompt submitted: the hook   │
│ runs loci suggest            │
└───────────────┬──────────────┘
                │ candidates injected
                ▼
┌──────────────────────────────┐
│ agent asks the loci CLI      │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ answer: location, note,      │
│ and the commit it describes  │
└───────────────┬──────────────┘
                │ note missing or stale
                ▼
┌──────────────────────────────┐
│ agent reads the code, writes │
│ the note through the CLI     │
└──────────────────────────────┘
```

其他的觸發點：

```text
checkout / switch ─────────> switch to that revision's memory
branch or commit dropped ──> prune what nothing can reach
hook missed ───────────────> rebuild on start, by command line or by skill
```

完整的規則請看[規格](doc/spec.zh-TW.md)。

## 原則

- **個人、本機。** 你的程式碼與它的記憶都留在你的機器上。
- **不擋路。** Loci 絕不會阻擋 git 操作，也不會讓它失敗。
- **由 hook 驅動。** Git 操作會立刻建立記憶，不會等到有人查詢才做。
- **全有或全無。** 一個 commit 的記憶要完整建好之後才看得到。
- **忠於 revision。** 答案描述的永遠是它所標明的那個 commit 的程式碼。
- **答案要小。** 回覆是一個位置加上一兩行，而不是整個檔案。

## 路線圖

- **Feature map。** 回答「哪些 function 參與了 X」。已放棄：在 gate 1 它沒有贏過全文搜尋。

## 文件

- [規格](doc/spec.zh-TW.md)——記憶的行為：function 與 note、建立、復原、清除與設定。
- [量測](doc/measurement.zh-TW.md)——速度與 token 用量要怎麼量測，以及讓專案結束的結果。

## 授權

[MIT](LICENSE)
