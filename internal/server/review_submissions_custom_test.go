package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCustomFields(t *testing.T) {
	valid := []customField{
		{ID: "height", Label: "Рост", Type: "chips", Options: []string{"150-160", "160-170"}},
		{ID: "fit", Label: "Как сидит", Type: "select", Options: []string{"маломерит", "в размер", "болтается"}, Required: true},
		{ID: "note", Label: "Заметка", Type: "text"},
	}
	out := normalizeCustomFields(valid)
	if len(out) != 3 {
		t.Fatalf("kept %d of 3 valid fields: %+v", len(out), out)
	}

	invalid := []customField{
		{ID: "", Label: "Нет id", Type: "chips", Options: []string{"a", "b"}},
		{ID: "nolabel", Label: "", Type: "chips", Options: []string{"a", "b"}},
		{ID: "badtype", Label: "Тип", Type: "radio", Options: []string{"a", "b"}},
		{ID: "dup", Label: "Дубль", Type: "chips", Options: []string{"a", "b"}},
		{ID: "dup", Label: "Дубль2", Type: "chips", Options: []string{"a", "b"}},
		{ID: "oneopt", Label: "Один вариант", Type: "chips", Options: []string{"a"}},
		{ID: "blank", Label: "Пустой", Type: "chips", Options: []string{"a", "  ", "b"}},
		{ID: "textone", Label: "Текст", Type: "text"},
	}
	// The maxCustomFields cap applies BEFORE filtering, so "blank" and
	// "textone" are truncated away; only the first "dup" survives.
	if got := normalizeCustomFields(invalid); len(got) != 1 || got[0].ID != "dup" {
		t.Fatalf("want [dup] after cap+filter, got %+v", got)
	}
	if got := normalizeCustomFields([]customField{
		{ID: "dup", Label: "Дубль", Type: "chips", Options: []string{"a", "b"}},
		{ID: "dup", Label: "Дубль2", Type: "chips", Options: []string{"a", "b"}},
		{ID: "oneopt", Label: "Один вариант", Type: "chips", Options: []string{"a"}},
		{ID: "blank", Label: "Пустой", Type: "chips", Options: []string{"a", "  ", "b"}},
		{ID: "textone", Label: "Текст", Type: "text"},
	}); len(got) != 3 || got[0].Options[0] != "a" || got[1].Options[1] != "b" || got[2].Type != "text" {
		t.Fatalf("filter without cap interference: %+v", got)
	}

	tooMany := make([]customField, 9)
	for i := range tooMany {
		tooMany[i] = customField{ID: string(rune('a' + i)), Label: "F", Type: "chips", Options: []string{"a", "b"}}
	}
	if got := normalizeCustomFields(tooMany); len(got) != maxCustomFields {
		t.Fatalf("cap at %d, got %d", maxCustomFields, len(got))
	}

	tooManyOptions := []customField{{ID: "x", Label: "X", Type: "chips", Options: strings.Split(strings.Repeat("v,", maxCustomOptions+3), ",")}}
	if got := normalizeCustomFields(tooManyOptions)[0].Options; len(got) != maxCustomOptions {
		t.Fatalf("options cap at %d, got %d", maxCustomOptions, len(got))
	}

	long := strings.Repeat("я", maxCustomStrLen+1)
	if got := normalizeCustomFields([]customField{{ID: long, Label: long, Type: "chips", Options: []string{"a", long}}}); len(got) != 0 {
		t.Fatalf("oversized strings must be dropped, got %+v", got)
	}
}

func TestCustomAnswersFromForm(t *testing.T) {
	fields := normalizeCustomFields([]customField{
		{ID: "height", Label: "Рост", Type: "chips", Options: []string{"150-160", "160-170"}, Required: true},
		{ID: "fit", Label: "Как сидит", Type: "select", Options: []string{"в размер", "болтается"}},
	})

	encoded, err := customAnswersFromForm(`{"height":"150-160","fit":"в размер","unknown":"x"}`, fields)
	if err != nil {
		t.Fatalf("valid answers: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(encoded), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got["height"] != "150-160" || got["fit"] != "в размер" {
		t.Fatalf("unexpected answers: %+v", got)
	}

	if _, err := customAnswersFromForm(`{"height":"bogus"}`, fields); err == nil {
		t.Fatal("off-menu option must be rejected")
	}
	if _, err := customAnswersFromForm(`{"fit":"в размер"}`, fields); err == nil {
		t.Fatal("missing required field must be rejected")
	}
	if _, err := customAnswersFromForm(`{"height":"150-160","fit":"junk"}`, fields); err == nil {
		t.Fatal("invalid option value must be rejected")
	}
	if _, err := customAnswersFromForm(`not-json`, fields); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}

	// No configured fields: anything is dropped, empty payload is fine.
	if out, err := customAnswersFromForm(`{"x":"y"}`, nil); err != nil || out != "" {
		t.Fatalf("no fields: out=%q err=%v", out, err)
	}
	if out, err := customAnswersFromForm(``, fields); err == nil || out != "" {
		t.Fatalf("missing required with empty payload: out=%q err=%v", out, err)
	}

	// Optional field omitted or blank is fine when others present.
	out, err := customAnswersFromForm(`{"height":"150-160"}`, fields)
	if err != nil {
		t.Fatalf("optional omitted: %v", err)
	}
	if !strings.Contains(out, "height") {
		t.Fatalf("missing height in %q", out)
	}
}
