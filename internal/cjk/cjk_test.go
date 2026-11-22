package cjk

import "testing"

func TestExpand(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"single character produces nothing", "型", ""},
		{"two characters", "模型", "模型"},
		{"overlapping bigrams", "大模型训练", "大模 模型 型训 训练"},
		{"fullwidth comma breaks the run", "模型，训练", "模型 ， 训练"},
		{"ideographic comma and fullwidth stop", "大模型、小模型。", "大模 模型 、 小模 模型 。"},
		{"ASCII punctuation and spaces break the run", "模型, 训练 评测", "模型 ,  训练   评测"},
		{"latin and digits are kept", "GPT4模型", "GPT4 模型"},
		{"mixed Chinese and English", "用 GPT 做模型训练 v2.0", " GPT  做模 模型 型训 训练  v2.0"},
		{"a single character next to latin has no bigram", "A型B", "A B"},
		{"emoji breaks the run", "模型😀训练", "模型 😀 训练"},
		{"hiragana and katakana", "ひらがなカタカナ", "ひら らが がな なカ カタ タカ カナ"},
		{"prolonged sound mark stays inside katakana", "コンピューター", "コン ンピ ピュ ュー ータ ター"},
		{"hangul", "한국어", "한국 국어"},
		{"han outside the BMP", "𠀀𠀁", "𠀀𠀁"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Expand(tt.in); got != tt.want {
				t.Errorf("Expand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSegments(t *testing.T) {
	tests := []struct {
		in   string
		want []Segment
	}{
		{"", nil},
		{"abc", []Segment{{"abc", false}}},
		{"模型", []Segment{{"模型", true}}},
		{"GPT模型x", []Segment{{"GPT", false}, {"模型", true}, {"x", false}}},
		{"模型，训练", []Segment{{"模型", true}, {"，", false}, {"训练", true}}},
		{"😀型", []Segment{{"😀", false}, {"型", true}}},
	}
	for _, tt := range tests {
		got := Segments(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("Segments(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("Segments(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}
