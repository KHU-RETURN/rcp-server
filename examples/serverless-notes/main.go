package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type httpEvent struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

type httpResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

type note struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

type dataRequest struct {
	Type    string `json:"$rcp"`
	Binding string `json:"binding"`
	SQL     string `json:"sql"`
	Params  []any  `json:"params"`
}

type dataReply struct {
	OK           bool             `json:"ok"`
	Error        string           `json:"error"`
	Rows         []map[string]any `json:"rows"`
	RowsAffected int64            `json:"rows_affected"`
}

type app struct {
	in  *bufio.Scanner
	out *json.Encoder
}

func (a *app) database(statement string, params ...any) (dataReply, error) {
	request := dataRequest{Type: "sql", Binding: "DB", SQL: statement, Params: params}
	if err := a.out.Encode(request); err != nil {
		return dataReply{}, err
	}
	if !a.in.Scan() {
		return dataReply{}, errors.New("database reply missing")
	}
	var reply dataReply
	if err := json.Unmarshal(a.in.Bytes(), &reply); err != nil {
		return dataReply{}, err
	}
	if !reply.OK {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}

func (a *app) respond(status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		encoded = []byte(`{"error":"response encoding failed"}`)
		status = 500
	}
	_ = a.out.Encode(httpResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json; charset=utf-8"},
		Body:       string(encoded),
	})
}

func (a *app) run(event httpEvent) {
	switch {
	case event.Method == "GET" && event.Path == "/notes":
		reply, err := a.database("SELECT id, text, created_at FROM notes ORDER BY id LIMIT 100")
		if err != nil {
			a.respond(500, map[string]string{"error": "메모를 불러오지 못했습니다."})
			return
		}
		notes := make([]note, 0, len(reply.Rows))
		for _, row := range reply.Rows {
			id, _ := row["id"].(string)
			text, _ := row["text"].(string)
			createdAt, _ := row["created_at"].(string)
			notes = append(notes, note{ID: id, Text: text, CreatedAt: createdAt})
		}
		a.respond(200, map[string]any{"notes": notes})
	case event.Method == "POST" && event.Path == "/notes":
		var input struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(event.Body), &input); err != nil {
			a.respond(400, map[string]string{"error": "JSON 본문이 필요합니다."})
			return
		}
		input.Text = strings.TrimSpace(input.Text)
		if input.Text == "" || len([]byte(input.Text)) > 500 {
			a.respond(400, map[string]string{"error": "메모는 1~500바이트로 입력하세요."})
			return
		}
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			a.respond(500, map[string]string{"error": "메모 ID를 만들지 못했습니다."})
			return
		}
		// 역순 시각 ID를 사용해 최근 메모가 먼저 정렬되도록 합니다.
		id := fmt.Sprintf("%013d-%s", 9999999999999-time.Now().UnixMilli(), hex.EncodeToString(random[:]))
		entry := note{ID: id, Text: input.Text, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		if _, err := a.database("INSERT INTO notes (id, text, created_at) VALUES (?, ?, ?)", entry.ID, entry.Text, entry.CreatedAt); err != nil {
			a.respond(500, map[string]string{"error": "메모를 저장하지 못했습니다."})
			return
		}
		a.respond(201, entry)
	case event.Method == "DELETE" && strings.HasPrefix(event.Path, "/notes/"):
		id := strings.TrimPrefix(event.Path, "/notes/")
		if id == "" || strings.Contains(id, "/") {
			a.respond(400, map[string]string{"error": "메모 ID가 올바르지 않습니다."})
			return
		}
		reply, err := a.database("DELETE FROM notes WHERE id = ?", id)
		if err != nil || reply.RowsAffected == 0 {
			a.respond(404, map[string]string{"error": "메모를 찾지 못했습니다."})
			return
		}
		a.respond(200, map[string]bool{"deleted": true})
	default:
		a.respond(404, map[string]string{"error": "지원하지 않는 경로입니다."})
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 4096), 256<<10)
	if !in.Scan() {
		return
	}
	a := app{in: in, out: json.NewEncoder(os.Stdout)}
	var event httpEvent
	if err := json.Unmarshal(in.Bytes(), &event); err != nil {
		a.respond(400, map[string]string{"error": "호출 형식이 올바르지 않습니다."})
		return
	}
	a.run(event)
}
