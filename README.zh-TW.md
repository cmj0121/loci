# Loci

> 每個任務都從乾淨的 context 開始，git 知道的事一樣都不丟。

[English](README.md) | 繁體中文

Agent 在同一個 session 裡連續做好幾個任務時，前面每個任務都還留在 context 裡。長 session 的花費，
大部分就花在這些被帶著走的 context 上：每一輪都要從 cache 再讀一次。Loci 會注意新任務什麼時候開始，
也就是切換到新的 git branch 的時候，並建議你執行 `/clear`。下一個 session 會拿到一份由 git 產生的
簡短 handoff，所以 git 知道的事都不會遺失。

```text
Loci: the branch changed from feat/gh-8380-scan-widget-elapsed to fix/gh-8388-crawler-qa-fixes
while the context is 297k tokens. This is a task boundary. Tell the user they can run /clear
now; Loci will hand the git state of fix/gh-8388-crawler-qa-fixes to the new session, so nothing
from git is lost.
```

在上面這個任務上，用 Loci 跑的 agent 花了 **$6.20 到 $6.55**，沒有 Loci 則是 **$10.88 到 $15.29**，
而且通過的測試相同。請看[量測紀錄](doc/handoff.zh-TW.md)。

## 為什麼需要

在一個產品 repository 上，agent 用掉的 token 有 97.8% 是 cache read：也就是被帶著走的 context，
每輪 125k 到 270k token，每一輪都重新讀一次。改程式碼本身不到 4%。cache read 每個 token 很便宜，
但每一輪都要付，所以 session 還留著越多先前的工作，花費就越高。

git branch 是 agent 自己建立的、很明確的任務邊界。Loci 就用它來判斷：

- **時機：** 切換到這個 session 還沒離開過的 branch，而且 context 很大。
- **內容：** 一份 2k token 以內的 handoff，完全由 git 產生。不呼叫模型，也不猜測。
- **方式：** Claude Code 的 hook。其他 agent 可以用 `loci handoff` 印出同樣的 handoff。

## 運作方式

```text
┌────────────┐  每個 shell 指令之後    ┌──────────────────────────────┐
│ Claude     │ ─────────────────────>  │ loci hook post-tool-use      │
│ Code       │                         │ 新 branch、context ≥ 100k？  │
│            │ <─────────────────────  │ 是：建議 /clear，            │
│            │  「可以執行 /clear」    │ 記下待交接的 handoff         │
│            │                         └──────────────┬───────────────┘
│            │                                        │ .git/loci/state.json
│            │  /clear 之後 session 開始              ▼
│            │ ─────────────────────>  ┌──────────────────────────────┐
│            │ <─────────────────────  │ loci hook session-start      │
│            │  git handoff            │ git：branch、issue、base、   │
└────────────┘                         │ commit、改動、狀態           │
                                       └──────────────────────────────┘
```

每個 shell 指令之後套用的規則：

| 切換到                                    | 判斷                        |
| ----------------------------------------- | --------------------------- |
| 這個 session 已經離開過的 branch          | revisit：任務進行中，不提示 |
| `main`、release branch、base 或 skip glob | long-lived：不提示          |
| 新 branch，context 低於門檻               | small：不提示               |
| 新 branch，context 達到或超過門檻         | 建議 `/clear`               |

handoff 會列出 repository、branch 和它的 issue、上一個 branch、base 和 merge-base、upstream、
從 base 以來的 commit 和改動的檔案，以及還沒 commit 的內容。它也會告訴新的 session：issue 的內文
和上面的決定不在它的 context 裡。

Loci 自己不會執行 `/clear`，由你決定。handoff 會等下一個清空後或全新的 session，最多等 24 小時。
hook 永遠不會讓 agent 失敗：錯誤都寫進 `.git/loci/log`。

## 安裝

Loci 是一個 Go binary。

```bash
make build                 # 產生 bin/loci
bin/loci install -C ~/src/my-repo
```

`loci install` 會在 repository 的 `.claude/settings.local.json` 加上兩個 hook，這個檔案不會分享給
團隊。其他設定都會保留。`loci install --uninstall` 會把它們移除。

## 指令

| 指令                               | 用途                                               |
| ---------------------------------- | -------------------------------------------------- |
| `loci handoff [--format md\|json]` | 印出目前 branch 的 git handoff，任何 agent 都能用  |
| `loci stats [--since 7d]`          | 統計切換、建議、handoff 的次數，以及採用率         |
| `loci replay`                      | 用規則重放這個 repository 過去的 session，估算節省 |
| `loci install [--uninstall]`       | 加上或移除 Claude Code 的 hook                     |
| `loci hook post-tool-use`          | PostToolUse hook（JSON 從 stdin 輸入）             |
| `loci hook session-start`          | SessionStart hook（JSON 從 stdin 輸入）            |

每個指令都接受 `-C <dir>`；`stats` 和 `replay` 接受 `--json`。

`loci replay` 會讀 `~/.claude/projects` 底下這個 repository 的 transcript，在你安裝之前，
就先算出這套規則能省多少：

```text
purpose         tokens now → with Loci    cost units now → with Loci
feature         509.5M → 280.1M (-45%)          61.2M → 38.7M (-37%)
bugfix          158.5M → 108.5M (-32%)          19.8M → 15.0M (-24%)
total           782.3M → 482.7M (-38%)          96.8M → 67.7M (-30%)
```

實際用了一週之後，`loci stats` 會告訴你這些建議有沒有被採用。

## 設定

所有設定都放在 git config，可以設在單一 repository，也可以設成 global。

| Key              | 預設值                                     | 意義                                     |
| ---------------- | ------------------------------------------ | ---------------------------------------- |
| `loci.threshold` | `100000`                                   | 切換時 context 達到多少 token 才 handoff |
| `loci.skip`      | `main`、`master`、`develop`、`release/*`…… | 其他長期 branch 的 glob（用 `--add`）    |
| `loci.base`      | `origin/main`、`origin/master`……           | handoff 用來比較的 base                  |

## 限制

- 省多少和切換時的 context 大小成正比：80k 時約 10%，300k 時約一半。
- 只在對話中決定、沒有寫進 git 或 issue tracker 的事，`/clear` 之後就不在了。
- 它讀的是 Claude Code 的 transcript 格式，這是內部格式，可能會改；讀不到 context 時，
  會當成 0，不做任何提示。

## 歷史

Loci 一開始是給 agent 用的本機程式碼記憶。那個假設已經量測過並結束；它的
[量測](doc/measurement.zh-TW.md)、[調查](doc/survey.zh-TW.md)和 [spec](doc/spec.zh-TW.md)
保留作為紀錄。

## 授權

請看 [LICENSE](LICENSE)。
