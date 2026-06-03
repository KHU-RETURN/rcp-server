package storage

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

const defaultObjectContentType = "application/octet-stream"

var smartQuoteReplacer = strings.NewReplacer(
	"\u2018", "",
	"\u2019", "",
	"\u201c", "",
	"\u201d", "",
)

func resolveObjectContentStream(r io.Reader, rawContentType, filename, objectName string) (io.Reader, string) {
	if contentType, ok := canonicalObjectContentType(rawContentType, false); ok {
		return r, contentType
	}
	if contentType, ok := contentTypeFromName(objectName); ok {
		return r, contentType
	}
	if contentType, ok := contentTypeFromName(filename); ok {
		return r, contentType
	}

	r, sniffedContentType, sniffed := sniffObjectContentType(r)
	if sniffed {
		if canonical, ok := canonicalObjectContentType(sniffedContentType, false); ok {
			return r, canonical
		}
		if canonical, ok := canonicalObjectContentType(sniffedContentType, true); ok {
			return r, canonical
		}
	}
	if contentType, ok := canonicalObjectContentType(rawContentType, true); ok {
		return r, contentType
	}
	return r, defaultObjectContentType
}

func canonicalObjectContentType(raw string, allowGeneric bool) (string, bool) {
	raw = strings.TrimSpace(smartQuoteReplacer.Replace(raw))
	if raw == "" {
		return "", false
	}

	mediaType, params, err := mime.ParseMediaType(raw)
	if err != nil || mediaType == "" {
		return "", false
	}

	mediaType = strings.ToLower(mediaType)
	if mediaType == defaultObjectContentType && !allowGeneric {
		return "", false
	}

	normalizedParams := make(map[string]string, len(params))
	for key, value := range params {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if key == "charset" {
			value = strings.Trim(value, `"`)
			if strings.EqualFold(value, "utf8") {
				value = "utf-8"
			} else {
				value = strings.ToLower(value)
			}
		}
		normalizedParams[key] = value
	}

	if mediaType == "text/html" {
		if _, ok := normalizedParams["charset"]; !ok {
			normalizedParams["charset"] = "utf-8"
		}
	}

	if contentType := mime.FormatMediaType(mediaType, normalizedParams); contentType != "" {
		return contentType, true
	}
	return "", false
}

func contentTypeFromName(name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "", false
	}
	if ext == ".html" || ext == ".htm" {
		return "text/html; charset=utf-8", true
	}
	return canonicalObjectContentType(mime.TypeByExtension(ext), false)
}

func sniffObjectContentType(r io.Reader) (io.Reader, string, bool) {
	if r == nil {
		return r, "", false
	}

	buf := make([]byte, 512)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return io.MultiReader(bytes.NewReader(buf[:n]), r), "", false
	}
	if n == 0 {
		return r, "", false
	}
	return io.MultiReader(bytes.NewReader(buf[:n]), r), http.DetectContentType(buf[:n]), true
}
