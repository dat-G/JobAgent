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

func TestChatAnswerPrefixFromJSONStream(t *testing.T) {
	streamed := `{"answer":"你的专业差距主要在工程化证据`
	if got := chatAnswerPrefixFromJSONStream(streamed); got != "你的专业差距主要在工程化证据" {
		t.Fatalf("unexpected partial answer: %q", got)
	}
	escaped := `{"answer":"第一行\n第二行","confidence":0.8}`
	if got := chatAnswerPrefixFromJSONStream(escaped); got != "第一行\n第二行" {
		t.Fatalf("unexpected escaped answer: %q", got)
	}
}

func TestBuildChatResponseFromPrestoOutput(t *testing.T) {
	output := `{"answer":"建议先补齐岗位证据。","conclusion":"证据短板最明显。","actions":["整理项目"],"confidence":0.8}`
	response, err := buildChatResponseFromPrestoOutput(output, "sess_1", "run_1")
	if err != nil {
		t.Fatalf("buildChatResponseFromPrestoOutput returned error: %v", err)
	}
	if response.Answer != "建议先补齐岗位证据。" {
		t.Fatalf("unexpected answer: %q", response.Answer)
	}
	if response.Payload["formatter"] != "presto_chat_workflow_answer_stream" {
		t.Fatalf("unexpected formatter: %#v", response.Payload["formatter"])
	}
}
