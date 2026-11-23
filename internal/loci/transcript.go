package loci

import (
	"bytes"
	"encoding/json"
	"os"
)

// The Claude Code transcript is an internal, unversioned JSONL format. Only the
// fields below are read; when they are missing the context counts as 0, so Loci
// stays quiet instead of guessing.
type usage struct {
	Input       int `json:"input_tokens"`
	CacheCreate int `json:"cache_creation_input_tokens"`
	CacheRead   int `json:"cache_read_input_tokens"`
	Output      int `json:"output_tokens"`
}

// prompt is the context the model read for this turn: input + cache write + cache read.
func (u usage) prompt() int { return u.Input + u.CacheCreate + u.CacheRead }

type record struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		ID    string `json:"id"`
		Usage *usage `json:"usage"`
	} `json:"message"`
}

const (
	tailChunk = 256 << 10 // first read from the end of the file
	tailMax   = 16 << 20  // give up past this much
)

// ContextTokens is the prompt size of the last main-chain assistant turn in a
// transcript. It reads the file from the end, so a 30 MB transcript costs one
// or two small reads.
func ContextTokens(path string) (int, error) { return lastPrompt(path, false) }

// SubagentTokens is the prompt size of the last assistant turn in a subagent's
// own transcript, whose turns are all marked as a sidechain.
func SubagentTokens(path string) (int, error) { return lastPrompt(path, true) }

// lastPrompt reads the last assistant turn with usage; sidechain turns count only when sidechain is set.
func lastPrompt(path string, sidechain bool) (int, error) {
	if path == "" {
		return 0, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := st.Size()
	for n := int64(tailChunk); ; n *= 2 {
		n = min(n, size)
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, size-n); err != nil {
			return 0, err
		}
		ls := bytes.Split(buf, []byte("\n"))
		first := 0
		if n < size {
			first = 1 // the first line is cut by the read window
		}
		for i := len(ls) - 1; i >= first; i-- {
			l := bytes.TrimSpace(ls[i])
			if len(l) == 0 || !bytes.Contains(l, []byte(`"usage"`)) {
				continue
			}
			var r record
			if json.Unmarshal(l, &r) != nil {
				continue
			}
			if r.Type == "assistant" && (sidechain || !r.IsSidechain) && r.Message.Usage != nil {
				return r.Message.Usage.prompt(), nil
			}
		}
		if n == size || n >= tailMax {
			return 0, nil
		}
	}
}
