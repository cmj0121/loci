# Loci

> 一份在本機、跟著 git 走的程式碼記憶。

[English](README.md) | 繁體中文

Loci 替你的程式碼保存一份記憶——東西在哪裡、它代表什麼——並且讓這份記憶與 git 保持同步。
AI coding agent 直接詢問 Loci，而不是搜尋整個 repository，因此能用少得多的 token
找到正確的檔案與行數。

## 為什麼需要

Agent 第一次進入一個 repository 時，大部分的預算都花在「找路」：grep、開檔、讀進遠超過
需要的內容，下一個 session 又把同樣的搜尋重做一遍。它學到的東西在 session 結束時就丟掉了。

Loci 把這件事變成一次查詢：

- **它在哪裡？** 以檔案與位置回答 function 在哪。
- **它是做什麼的？** 每個 function 一則簡短的 concept note，留給下一個 agent。
- **是哪個 revision？** 你目前 checkout 的那個 commit，不是過期的快照。

## 概念

```text
  ┌──────────────────┐  ┌──────────────────┐      ┌──────────────────────────┐
  │ git hooks        │  │ command line     │      │ agent                    │
  │ (per repository) │  │ e.g. rebuild     │      │ skill: when and how      │
  └────────┬─────────┘  └────────┬─────────┘      └──────────────────────────┘
           │                     │                             ▲
           │ git event           │ command                     │ ask, answer
           ▼                     ▼                             ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│ Loci                                                                         │
│ ┌────────────────────────────────────────┐  ┌──────────────────────────────┐ │
│ │ event receiver                         │  │ MCP server                   │ │
│ │ takes git events and commands,         │  │ finds functions and notes,   │ │
│ │ returns at once                        │  │ writes and updates notes     │ │
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
  檔案與位置——以及由 LLM 為每個 function 撰寫的一則 concept note。Loci 本身不呼叫 LLM。
- **Git-aware。** Git 操作就是觸發點。Commit 會建立它的記憶，checkout 會切換到該 revision
  的記憶，刪除 branch 或丟棄 commit 則會清除再也用不到的部分。
- **獨立的 service。** 一個個人、本機的 service 服務你所有的專案；有一份 global setup，
  每個 repository 可以用 `.loci` 檔覆寫。
- **由 agent 共享。** Skill 告訴 agent 什麼時候該問 Loci；MCP server 提供查詢與寫回 note
  的工具。一個 agent 弄懂的事，下一個 agent 一開始就知道。

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
│ agent asks through MCP       │
└───────────────┬──────────────┘
                │
                ▼
┌──────────────────────────────┐
│ answer: location, note,      │
│ and the commit it describes  │
└───────────────┬──────────────┘
                │ note needs update
                ▼
┌──────────────────────────────┐
│ agent reads the code, writes │
│ the new note through MCP     │
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

## 文件

- [規格](doc/spec.zh-TW.md)——記憶的行為：function 與 note、建立、復原、清除與設定。
- [量測](doc/measurement.zh-TW.md)——速度與 token 用量要怎麼量測。

## 授權

[MIT](LICENSE)
