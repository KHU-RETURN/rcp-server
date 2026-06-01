package storage

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/KHU-RETURN/rcp-server/internal/api"
)

const defaultObjectContentType = "application/octet-stream"

var smartQuoteReplacer = strings.NewReplacer(
	"\u2018", "",
	"\u2019", "",
	"\u201c", "",
	"\u201d", "",
)

func resolveObjectContentType(file multipart.File, header *multipart.FileHeader, objectName string) string {
	var rawContentType, filename string
	if header != nil {
		rawContentType = header.Header.Get(api.HeaderContentType)
		filename = header.Filename
	}

	if contentType, ok := canonicalObjectContentType(rawContentType, false); ok {
		return contentType
	}
	if contentType, ok := contentTypeFromName(objectName); ok {
		return contentType
	}
	if contentType, ok := contentTypeFromName(filename); ok {
		return contentType
	}
	if contentType, ok := sniffObjectContentType(file); ok {
		if canonical, ok := canonicalObjectContentType(contentType, false); ok {
			return canonical
		}
		if canonical, ok := canonicalObjectContentType(contentType, true); ok {
			return canonical
		}
	}
	if contentType, ok := canonicalObjectContentType(rawContentType, true); ok {
		return contentType
	}
	return defaultObjectContentType
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

func sniffObjectContentType(file multipart.File) (string, bool) {
	if file == nil {
		return "", false
	}

	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", false
	}

	buf := make([]byte, 512)
	n, readErr := file.Read(buf)
	if _, seekErr := file.Seek(position, io.SeekStart); seekErr != nil {
		return "", false
	}
	if readErr != nil && readErr != io.EOF {
		return "", false
	}
	if n == 0 {
		return "", false
	}
	return http.DetectContentType(buf[:n]), true
}
