package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func decodePayload(r *http.Request) (map[string]string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	mediaType := ""
	if contentType != "" {
		mt, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, err
		}
		mediaType = strings.ToLower(mt)
	}

	decodeOrder := []string{"form", "multipart", "json"}
	switch mediaType {
	case "application/x-www-form-urlencoded":
		decodeOrder = []string{"form", "multipart", "json"}
	case "multipart/form-data":
		decodeOrder = []string{"multipart", "form", "json"}
	case "application/json":
		decodeOrder = []string{"json", "form", "multipart"}
	case "":
		// Default to form when Content-Type is missing.
		decodeOrder = []string{"form", "multipart", "json"}
	default:
		// Unknown content type, still do best-effort attempts.
		decodeOrder = []string{"form", "multipart", "json"}
	}

	var errs []error
	for _, decoder := range decodeOrder {
		switch decoder {
		case "form":
			data, err := decodeForm(body)
			if err == nil {
				return data, nil
			}
			errs = append(errs, fmt.Errorf("form: %w", err))
		case "multipart":
			data, err := decodeMultipart(body, contentType)
			if err == nil {
				return data, nil
			}
			errs = append(errs, fmt.Errorf("multipart: %w", err))
		case "json":
			data, err := decodeJSON(body)
			if err == nil {
				return data, nil
			}
			errs = append(errs, fmt.Errorf("json: %w", err))
		}
	}

	return nil, errors.Join(errs...)
}

func decodeForm(body []byte) (map[string]string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return map[string]string{}, nil
	}
	if bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("[")) {
		return nil, errors.New("body does not look like form payload")
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}

	data := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) == 0 {
			continue
		}
		data[key] = items[0]
	}
	return data, nil
}

func decodeMultipart(body []byte, contentType string) (map[string]string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}

	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, errors.New("missing multipart boundary")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = form.RemoveAll()
	}()

	data := make(map[string]string, len(form.Value))
	for key, items := range form.Value {
		if len(items) == 0 {
			continue
		}
		data[key] = items[0]
	}
	return data, nil
}

func decodeJSON(body []byte) (map[string]string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return map[string]string{}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()

	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}

	// Reject trailing tokens to avoid silently accepting malformed payloads.
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, errors.New("invalid json payload")
	} else if !errors.Is(err, io.EOF) {
		return nil, err
	}

	data := make(map[string]string, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case nil:
			continue
		case string:
			data[key] = v
		case bool:
			data[key] = strconv.FormatBool(v)
		case json.Number:
			data[key] = v.String()
		default:
			return nil, fmt.Errorf("json field %q must be scalar", key)
		}
	}
	return data, nil
}
