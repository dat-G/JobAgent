package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSplitChatAnswerChunksPreservesAnswer(t *testing.T) {
	answer := "你的专业维度目前得分较高，但岗位表达和工程化测试证据仍需要补强。建议先整理作品集，再补 Playwright 报告。"
	chunks := splitChatAnswerChunks(answer)
	if len(chunks) < 2 {
		t.Fatalf("expected streamed answer to be split into multiple chunks, got %d", len(chunks))
	}
	if strings.Join(chunks, "") != answer {
		t.Fatalf("chunks do not reconstruct answer: %#v", chunks)
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 28 {
			t.Fatalf("chunk too large: %q", chunk)
		}
	}
}

func TestWriteChatSSE(t *testing.T) {
	var buffer bytes.Buffer
	if err := writeChatSSE(&buffer, "chat.chunk", map[string]any{"delta": "你好"}); err != nil {
		t.Fatalf("writeChatSSE returned error: %v", err)
	}
	text := buffer.String()
	if !strings.Contains(text, "event: chat.chunk\n") {
		t.Fatalf("missing SSE event name: %q", text)
	}
	if !strings.Contains(text, `data: {"delta":"你好"}`) {
		t.Fatalf("missing SSE JSON payload: %q", text)
	}
	if !strings.HasSuffix(text, "\n\n") {
		t.Fatalf("SSE event must end with a blank line: %q", text)
	}
}
