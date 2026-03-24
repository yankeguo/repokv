package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDecodePayload_FormURLEncoded(t *testing.T) {
	form := url.Values{}
	form.Set("foo", "bar")
	form.Set("num", "1")

	req := httptest.NewRequest(http.MethodPost, "/repo", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	data, err := decodePayload(req)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if data["foo"] != "bar" {
		t.Fatalf("unexpected foo: %q", data["foo"])
	}
	if data["num"] != "1" {
		t.Fatalf("unexpected num: %q", data["num"])
	}
}

func TestDecodePayload_MultipartForm(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("hello", "world"); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.WriteField("count", "2"); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/repo", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())

	data, err := decodePayload(req)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if data["hello"] != "world" {
		t.Fatalf("unexpected hello: %q", data["hello"])
	}
	if data["count"] != "2" {
		t.Fatalf("unexpected count: %q", data["count"])
	}
}

func TestDecodePayload_JSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/repo", bytes.NewBufferString(`{"a":"b","n":3,"ok":true}`))
	req.Header.Set("Content-Type", "application/json")

	data, err := decodePayload(req)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if data["a"] != "b" {
		t.Fatalf("unexpected a: %q", data["a"])
	}
	if data["n"] != "3" {
		t.Fatalf("unexpected n: %q", data["n"])
	}
	if data["ok"] != "true" {
		t.Fatalf("unexpected ok: %q", data["ok"])
	}
}

func TestDecodePayload_DefaultFormWhenMissingContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/repo", bytes.NewBufferString("k=v&x=y"))
	req.Header.Del("Content-Type")

	data, err := decodePayload(req)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if data["k"] != "v" {
		t.Fatalf("unexpected k: %q", data["k"])
	}
	if data["x"] != "y" {
		t.Fatalf("unexpected x: %q", data["x"])
	}
}

func TestDecodePayload_FallbackToJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/repo", bytes.NewBufferString(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	data, err := decodePayload(req)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if data["foo"] != "bar" {
		t.Fatalf("unexpected foo: %q", data["foo"])
	}
}
