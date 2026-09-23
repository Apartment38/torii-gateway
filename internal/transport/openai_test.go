package transport

import "testing"

func TestRequestModelRejectsTopLevelTrailingComma(t *testing.T) {
	if _, err := RequestModel([]byte(`{"model":"local/a",}`)); err == nil {
		t.Fatal("RequestModel() accepted a trailing comma")
	}
}

func TestRequestModelPreservesUnknownFields(t *testing.T) {
	model, err := RequestModel([]byte(`{"future":{"value":true},"model":"local/a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if model != "local/a" {
		t.Fatalf("RequestModel() = %q, want local/a", model)
	}
}
